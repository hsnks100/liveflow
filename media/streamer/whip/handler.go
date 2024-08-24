package whip

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/intervalpli"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v3"
	gomp4 "github.com/yapingcat/gomedia/go-mp4"

	"mrw-clone/log"
	"mrw-clone/media/hub"
)

var (
	errNoStreamKey = echo.NewHTTPError(http.StatusUnauthorized, "No stream key provided")
)

type WHIP struct {
	hub    *hub.Hub
	tracks map[string][]*webrtc.TrackLocalStaticRTP
}

type WHIPArgs struct {
	Hub *hub.Hub
}

var (
	//videoTrack *webrtc.TrackLocalStaticRTP
	//audioTrack *webrtc.TrackLocalStaticRTP

	peerConnectionConfiguration = webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
)

func NewWHIP(args WHIPArgs) *WHIP {
	return &WHIP{
		hub:    args.Hub,
		tracks: make(map[string][]*webrtc.TrackLocalStaticRTP),
	}
}

func (r *WHIP) Serve() {
	whipServer := echo.New()
	whipServer.Static("/", ".")
	whipServer.POST("/whip", r.whipHandler)
	whipServer.POST("/whep", r.whepHandler)
	//whipServer.PATCH("/whip", whipHandler)
	whipServer.Start(":5555")
}

func (r *WHIP) bearerToken(c echo.Context) (string, error) {
	bearerToken := c.Request().Header.Get("Authorization")
	if len(bearerToken) == 0 {
		return "", errNoStreamKey
	}
	authHeaderParts := strings.Split(bearerToken, " ")
	if len(authHeaderParts) != 2 {
		return "", errNoStreamKey
	}
	return authHeaderParts[1], nil
}
func (r *WHIP) whipHandler(c echo.Context) error {
	// Read the offer from HTTP Request
	offer, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	streamKey, err := r.bearerToken(c)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}
	fmt.Println("streamkey: ", streamKey)

	// Create a MediaEngine object to configure the supported codec
	m := &webrtc.MediaEngine{}

	// Setup the codecs you want to use.
	if err = m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, Channels: 0, SDPFmtpLine: "", RTCPFeedback: nil},
		PayloadType:        96,
	}, webrtc.RTPCodecTypeVideo); err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}
	if err = m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2, SDPFmtpLine: "", RTCPFeedback: nil},
		PayloadType:        111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	// Create a InterceptorRegistry
	i := &interceptor.Registry{}

	// Register a intervalpli factory
	intervalPliFactory, err := intervalpli.NewReceiverInterceptor()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}
	i.Add(intervalPliFactory)

	// Use the default set of Interceptors
	if err = webrtc.RegisterDefaultInterceptors(m, i); err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	// Create the API object with the MediaEngine
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(i))

	// Create a new RTCPeerConnection
	peerConnection, err := api.NewPeerConnection(peerConnectionConfiguration)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	// Allow us to receive 1 video track
	if _, err = peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo); err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}
	// Allow us to receive 1 audio track
	if _, err = peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio); err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	ctx := context.Background()
	//webrtcHandler := NewWebRTCHandler()
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "pion")
	if err != nil {
		panic(err)
	}
	// Add Audio Track that is being written to from WHIP Session
	audioTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion")
	if err != nil {
		panic(err)
	}
	if _, ok := r.tracks[streamKey]; !ok {
		r.tracks[streamKey] = []*webrtc.TrackLocalStaticRTP{videoTrack, audioTrack}
	}
	// Set a handler for when a new remote track starts
	mp4File, err := os.CreateTemp("./", "*.mp4")
	if err != nil {
		return err
	}

	muxer, err := gomp4.CreateMp4Muxer(mp4File)
	if err != nil {
		return err
	}
	whipHandler := NewWebRTCHandler(muxer, mp4File)
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("ICE Connection State has changed: %s\n", connectionState.String())
		switch connectionState {
		case webrtc.ICEConnectionStateConnected:
			r.hub.Notify(streamKey)
			fmt.Println("ICE Connection State Connected")
		case webrtc.ICEConnectionStateDisconnected:
			whipHandler.OnClose(ctx)
			delete(r.tracks, streamKey)
			fmt.Println("ICE Connection State Disconnected")
		case webrtc.ICEConnectionStateFailed:
			fmt.Println("ICE Connection State Failed")
			_ = peerConnection.Close()
		}
	})
	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		fmt.Printf("Track has started, of type %s\n", track.Kind())
		var videoPackets []*rtp.Packet
		var audioPackets []*rtp.Packet
		currentVideoTimestamp := uint32(0)
		currentAudioTimestamp := uint32(0)
		for {
			pkt, _, err := track.ReadRTP()
			if err != nil {
				log.Error(ctx, err, "failed to read rtp")
				break
			}

			switch track.Kind() {
			case webrtc.RTPCodecTypeVideo:
				//fmt.Println("timestamp: ", pkt.Timestamp)
				if len(videoPackets) > 0 && currentVideoTimestamp != pkt.Timestamp {
					whipHandler.OnVideo(ctx, videoPackets)
					videoPackets = nil
				}

				videoPackets = append(videoPackets, pkt)
				currentVideoTimestamp = pkt.Timestamp
				if pkt.Marker {
					whipHandler.OnVideo(ctx, videoPackets)
					videoPackets = nil
				}
				//fmt.Println("frame len: ", len(h264Bytes))
				if err = videoTrack.WriteRTP(pkt); err != nil {
					panic(err)
				}
			case webrtc.RTPCodecTypeAudio:
				if len(audioPackets) > 0 && currentAudioTimestamp != pkt.Timestamp {
					whipHandler.OnAudio(ctx, audioPackets)
					audioPackets = nil
				}
				audioPackets = append(audioPackets, pkt)
				currentAudioTimestamp = pkt.Timestamp
				if pkt.Marker {
					whipHandler.OnAudio(ctx, audioPackets)
					audioPackets = nil
				}
				if err = audioTrack.WriteRTP(pkt); err != nil {
					panic(err)
				}
			}
		}
	})
	// Send answer via HTTP Response
	return writeAnswer3(c, peerConnection, offer, "/whip")

}

type WebRTCHandler struct {
	hub        *hub.Hub
	muxer      *gomp4.Movmuxer
	file       *os.File
	videoIndex uint32
}

func NewWebRTCHandler(muxer *gomp4.Movmuxer, file *os.File, hub *hub.Hub) *WebRTCHandler {
	ret := &WebRTCHandler{}
	ret.file = file
	ret.muxer = muxer
	ret.hub = hub
	ret.videoIndex = muxer.AddVideoTrack(gomp4.MP4_CODEC_H264)
	return ret
}

func (w *WebRTCHandler) OnClose(ctx context.Context) error {
	fmt.Println("OnClose")
	w.muxer.WriteTrailer()
	w.file.Close()
	return nil
}

func (w *WebRTCHandler) OnVideo(ctx context.Context, packets []*rtp.Packet) error {
	var h264RTPParser = &codecs.H264Packet{}
	payload := make([]byte, 0)
	for _, pkt := range packets {
		b, err := h264RTPParser.Unmarshal(pkt.Payload)
		if err != nil {
			log.Error(ctx, err, "failed to unmarshal h264")
		}
		payload = append(payload, b...)
	}

	h264Bytes := payload
	if len(h264Bytes) > 0 {
		timestamp := packets[0].Timestamp
		err := w.muxer.Write(w.videoIndex, h264Bytes, uint64(timestamp)/90, uint64(timestamp)/90)
		if err != nil {
			log.Error(ctx, err, "failed to write h264")
			return err
		}
	}
	return nil
}

func (w *WebRTCHandler) OnAudio(ctx context.Context, packets []*rtp.Packet) error {
	return nil
}

func (r *WHIP) whepHandler(c echo.Context) error {
	// Read the offer from HTTP Request
	offer, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}
	streamKey, err := r.bearerToken(c)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	// Create a new RTCPeerConnection
	peerConnection, err := webrtc.NewPeerConnection(peerConnectionConfiguration)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	var rtpSenders []*webrtc.RTPSender
	for _, track := range r.tracks[streamKey] {
		sender, err := peerConnection.AddTrack(track)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, err.Error())
		}
		rtpSenders = append(rtpSenders, sender)
	}

	// Read incoming RTCP packets
	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			for _, rtpSender := range rtpSenders {
				if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
					return
				}
			}
		}
	}()
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("ICE Connection State has changed: %s\n", connectionState.String())

		if connectionState == webrtc.ICEConnectionStateFailed {
			_ = peerConnection.Close()
		}
	})
	// Send answer via HTTP Response
	return writeAnswer3(c, peerConnection, offer, "/whep")
}
func writeAnswer3(c echo.Context, peerConnection *webrtc.PeerConnection, offer []byte, path string) error {
	// Set the handler for ICE connection state

	if err := peerConnection.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: string(offer)}); err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	// Create channel that is blocked until ICE Gathering is complete
	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

	// Create answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	} else if err = peerConnection.SetLocalDescription(answer); err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	// Block until ICE Gathering is complete, disabling trickle ICE
	<-gatherComplete

	// WHIP+WHEP expects a Location header and a HTTP Status Code of 201
	c.Response().Header().Add("Location", path)
	c.Response().WriteHeader(http.StatusCreated)

	// Write Answer with Candidates as HTTP Response
	return c.String(http.StatusOK, peerConnection.LocalDescription().SDP)
}
