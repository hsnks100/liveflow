package whip

import (
	"context"
	"fmt"
	"io"
	"liveflow/log"
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
	hub        *hub.Hub
	tracks     map[string][]*webrtc.TrackLocalStaticRTP
	dockerMode bool
	port       int
}

type WHIPArgs struct {
	Hub        *hub.Hub
	Tracks     map[string][]*webrtc.TrackLocalStaticRTP
	DockerMode bool
	Port       int
}

func NewWHIP(args WHIPArgs) *WHIP {
	return &WHIP{
		hub:        args.Hub,
		tracks:     args.Tracks,
		dockerMode: args.DockerMode,
		port:       args.Port,
	}
}

func (r *WHIP) Serve() {
	whipServer := echo.New()
	whipServer.HideBanner = true
	whipServer.Static("/", ".")
	whipServer.POST("/whip", r.whipHandler)
	whipServer.POST("/whep", r.whepHandler)
	//whipServer.PATCH("/whip", whipHandler)
	whipServer.Start(":" + strconv.Itoa(r.port))
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
	ctx := context.Background()
	// Read the offer from HTTP Request
	offer, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	// Parse the SDP
	parsedSDP := sdp.SessionDescription{}
	if err := parsedSDP.Unmarshal([]byte(offer)); err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	// Count the number of media tracks
	trackCount := 0
	for _, media := range parsedSDP.MediaDescriptions {
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

	err = registerCodec(m)
	if err != nil {
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
	se := webrtc.SettingEngine{}
	se.SetEphemeralUDPPortRange(30000, 30500)
	if r.dockerMode {
		se.SetNAT1To1IPs([]string{"127.0.0.1"}, webrtc.ICECandidateTypeHost)
		se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(i), webrtc.WithSettingEngine(se))

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
			log.Info(ctx, "OnTrack", "Codec: ", t.MimeType)
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
