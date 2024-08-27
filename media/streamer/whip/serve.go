package whip

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/intervalpli"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v3"

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

	whipHandler := NewWebRTCHandler(r.hub, &WebRTCHandlerArgs{
		Hub:            r.hub,
		PeerConnection: peerConnection,
		StreamID:       streamKey,
		Tracks:         r.tracks,
	})
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		whipHandler.OnICEConnectionStateChange(connectionState)
		fmt.Printf("ICE Connection State has changed: %s\n", connectionState.String())
	})
	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		whipHandler.OnTrack(track, receiver)
	})
	// Send answer via HTTP Response
	return writeAnswer3(c, peerConnection, offer, "/whip")
}

type WebRTCHandler struct {
	hub          *hub.Hub
	pc           *webrtc.PeerConnection
	streamID     string
	timestampGen TimestampGenerator[int64]
	tracks       map[string][]*webrtc.TrackLocalStaticRTP
	videoTrack   *webrtc.TrackLocalStaticRTP
	audioTrack   *webrtc.TrackLocalStaticRTP
}

type WebRTCHandlerArgs struct {
	Hub            *hub.Hub
	PeerConnection *webrtc.PeerConnection
	StreamID       string
	Tracks         map[string][]*webrtc.TrackLocalStaticRTP
}

func NewWebRTCHandler(hub *hub.Hub, args *WebRTCHandlerArgs) *WebRTCHandler {
	//ctx := context.Background()
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "pion")
	if err != nil {
		panic(err)
	}
	// Add Audio Track that is being written to from WHIP Session
	audioTrack, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion")
	if err != nil {
		panic(err)
	}
	//if _, ok := r.tracks[streamKey]; !ok {
	//	r.tracks[streamKey] = []*webrtc.TrackLocalStaticRTP{videoTrack, audioTrack}
	//}
	ret := &WebRTCHandler{}
	ret.hub = hub
	ret.streamID = args.StreamID
	ret.timestampGen = TimestampGenerator[int64]{}
	ret.pc = args.PeerConnection
	ret.tracks = args.Tracks
	if _, ok := ret.tracks[args.StreamID]; !ok {
		ret.tracks[args.StreamID] = []*webrtc.TrackLocalStaticRTP{videoTrack, audioTrack}
	}
	ret.videoTrack = videoTrack
	ret.audioTrack = audioTrack
	return ret
}

func (w *WebRTCHandler) OnICEConnectionStateChange(connectionState webrtc.ICEConnectionState) {
	ctx := context.Background()
	switch connectionState {
	case webrtc.ICEConnectionStateConnected:
		w.hub.Notify(w.streamID)
		fmt.Println("ICE Connection State Connected")
	case webrtc.ICEConnectionStateDisconnected:
		w.OnClose(ctx)
		//delete(w.tracks, streamKey)
		fmt.Println("ICE Connection State Disconnected")
	case webrtc.ICEConnectionStateFailed:
		fmt.Println("ICE Connection State Failed")
		_ = w.pc.Close()
	}
}

func (w *WebRTCHandler) OnTrack(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
	ctx := context.Background()
	fmt.Printf("Track has started, of type %s %s\n", track.Kind(), track.Codec().MimeType)
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
				w.OnVideo(ctx, videoPackets)
				videoPackets = nil
			}

			videoPackets = append(videoPackets, pkt)
			currentVideoTimestamp = pkt.Timestamp
			if pkt.Marker {
				w.OnVideo(ctx, videoPackets)
				videoPackets = nil
			}
			//fmt.Println("frame len: ", len(h264Bytes))
			if err = w.videoTrack.WriteRTP(pkt); err != nil {
				panic(err)
			}
		case webrtc.RTPCodecTypeAudio:
			if len(audioPackets) > 0 && currentAudioTimestamp != pkt.Timestamp {
				w.OnAudio(ctx, audioPackets)
				audioPackets = nil
			}
			audioPackets = append(audioPackets, pkt)
			currentAudioTimestamp = pkt.Timestamp
			if pkt.Marker {
				w.OnAudio(ctx, audioPackets)
				audioPackets = nil
			}
			if err = w.audioTrack.WriteRTP(pkt); err != nil {
				panic(err)
			}
		}
	}

}
func (w *WebRTCHandler) OnClose(ctx context.Context) error {
	w.hub.Unpublish(w.streamID)
	fmt.Println("OnClose")
	return nil
}

func (w *WebRTCHandler) OnVideo(ctx context.Context, packets []*rtp.Packet) error {
	var h264RTPParser = &codecs.H264Packet{}
	payload := make([]byte, 0)
	for _, pkt := range packets {
		if len(pkt.Payload) == 0 {
			continue
		}
		b, err := h264RTPParser.Unmarshal(pkt.Payload)
		if err != nil {
			log.Error(ctx, err, "failed to unmarshal h264")
		}
		payload = append(payload, b...)
	}

	if len(payload) == 0 {
		return nil
	}
	pts := w.timestampGen.GetTimestamp(int64(packets[0].Timestamp))
	w.hub.Publish(w.streamID, &hub.FrameData{
		H264Video: &hub.H264Video{
			PTS:            pts,
			DTS:            pts,
			VideoClockRate: 90000,
			Data:           payload,
			SPS:            nil,
			PPS:            nil,
			SliceType:      0,
			CodecData:      nil,
		},
		AACAudio: nil,
	})

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
