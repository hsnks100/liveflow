package whip

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/intervalpli"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"

	"liveflow/media/hub"
)

var (
	errNoStreamKey = echo.NewHTTPError(http.StatusUnauthorized, "No stream key provided")
)

var (
	peerConnectionConfiguration = webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
)

type WHIP struct {
	hub    *hub.Hub
	tracks map[string][]*webrtc.TrackLocalStaticRTP
}

type WHIPArgs struct {
	Hub    *hub.Hub
	Tracks map[string][]*webrtc.TrackLocalStaticRTP
}

func NewWHIP(args WHIPArgs) *WHIP {
	return &WHIP{
		hub:    args.Hub,
		tracks: args.Tracks,
	}
}

func (r *WHIP) Serve() {
	whipServer := echo.New()
	whipServer.HideBanner = true
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

	// Parse the SDP
	parsedSDP := sdp.SessionDescription{}
	if err := parsedSDP.Unmarshal([]byte(offer)); err != nil {
		panic(err)
	}

	// Count the number of media tracks
	trackCount := 0
	for _, media := range parsedSDP.MediaDescriptions {
		//for _, attr := range media.Attributes {
		//	if attr.Key == "rtpmap" {
		//		rtpmap, err := parseRTPMAP(attr)
		//		if err != nil {
		//			return c.JSON(http.StatusInternalServerError, err.Error())
		//		}
		//		fmt.Printf("RTPMAP: %+v\n", rtpmap)
		//	}
		//}
		if media.MediaName.Media == "audio" || media.MediaName.Media == "video" {
			trackCount++
		}
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
		Hub:                r.hub,
		PeerConnection:     peerConnection,
		StreamID:           streamKey,
		ExpectedTrackCount: trackCount,
	})
	trackArgCh := make(chan TrackArgs)
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		whipHandler.OnICEConnectionStateChange(connectionState, trackArgCh)
		fmt.Printf("ICE Connection State has changed: %s\n", connectionState.String())
	})
	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		for _, t := range receiver.GetParameters().Codecs {
			fmt.Println("Codec: ", t.MimeType)
		}
		whipHandler.OnTrack(track, receiver, trackArgCh)
	})
	// Send answer via HTTP Response
	return writeAnswer3(c, peerConnection, offer, "/whip")
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

type RTPMAP struct {
	PayloadType webrtc.PayloadType
	MimeType    string
	CodecName   string
	ClockRate   uint32
	Channels    uint16
}

func parseRTPMAP(attr sdp.Attribute) (RTPMAP, error) {
	const (
		attributeRtpMap     = "rtpmap"
		attributeValueIndex = 2
		fmtpIndexCodec      = 0
		fmtpIndexClockRate  = 1
		fmtpIndexChannels   = 2
	)
	var (
		InvalidSDPAttribute = fmt.Errorf("invalid SDP attribute")
	)
	if attr.Key != attributeRtpMap || attr.Value == "" {
		return RTPMAP{}, InvalidSDPAttribute
	}
	var rtpmap RTPMAP
	r := strings.Split(attr.Value, " ")
	if len(r) >= 1 {
		pt, err := strconv.Atoi(r[0])
		if err != nil {
			return RTPMAP{}, InvalidSDPAttribute
		}
		rtpmap.PayloadType = webrtc.PayloadType(pt)
	}
	if len(r) >= attributeValueIndex {
		rtpmap.MimeType = r[1]
		r2 := strings.Split(rtpmap.MimeType, "/")
		if len(r2) >= fmtpIndexCodec+1 {
			rtpmap.CodecName = r2[fmtpIndexCodec]
		}
		if len(r2) >= fmtpIndexClockRate+1 {
			if clockRate, err := strconv.Atoi(r2[fmtpIndexClockRate]); err == nil {
				rtpmap.ClockRate = uint32(clockRate)
			}
		}
		if len(r2) >= fmtpIndexChannels+1 {
			if channels, err := strconv.Atoi(r2[fmtpIndexChannels]); err == nil {
				rtpmap.Channels = uint16(channels)
			}
		}
	}
	return rtpmap, nil
}
