package whip

import (
	"context"
	"fmt"
	"io"
	"liveflow/media/streamer/ingress"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v3"

	"liveflow/log"
	"liveflow/media/hub"
)

var (
	ErrMissingTrack     = fmt.Errorf("missing track")
	ErrTrackWaitTimeOut = fmt.Errorf("track wait timeout")
)

type WebRTCHandler struct {
	hub               *hub.Hub
	pc                *webrtc.PeerConnection
	streamID          string
	audioTimestampGen TimestampGenerator[int64]
	videoTimestampGen TimestampGenerator[int64]
	notifiedSource    bool

	mediaArgs          []hub.MediaSpec
	expectedTrackCount int
}

func (w *WebRTCHandler) Depth() int {
	return 0
}

type WebRTCHandlerArgs struct {
	Hub                *hub.Hub
	PeerConnection     *webrtc.PeerConnection
	StreamID           string
	Tracks             map[string][]*webrtc.TrackLocalStaticRTP
	ExpectedTrackCount int
}

func NewWebRTCHandler(hub *hub.Hub, args *WebRTCHandlerArgs) *WebRTCHandler {
	ret := &WebRTCHandler{
		hub:                hub,
		streamID:           args.StreamID,
		audioTimestampGen:  TimestampGenerator[int64]{},
		videoTimestampGen:  TimestampGenerator[int64]{},
		pc:                 args.PeerConnection,
		expectedTrackCount: args.ExpectedTrackCount,
	}
	return ret
}
func (w *WebRTCHandler) StreamID() string {
	return w.streamID
}

func (w *WebRTCHandler) Name() string {
	return "webrtc"
}

func (w *WebRTCHandler) MediaSpecs() []hub.MediaSpec {
	var ret []hub.MediaSpec
	for _, arg := range w.mediaArgs {
		ret = append(ret, arg)
	}
	return ret
}

func (w *WebRTCHandler) WaitTrackArgs(ctx context.Context, timeout time.Duration, trackArgCh <-chan TrackArgs) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			if len(w.mediaArgs) == 0 {
				return ErrMissingTrack
			}
			return ErrTrackWaitTimeOut
		case args := <-trackArgCh:
			audioSplits := strings.Split(args.MimeType, "audio/")
			videoSplits := strings.Split(args.MimeType, "video/")
			if len(audioSplits) > 1 {
				w.mediaArgs = append(w.mediaArgs, hub.MediaSpec{
					MediaType: hub.Audio,
					ClockRate: args.ClockRate,
					CodecType: hub.CodecType(strings.ToLower(audioSplits[1])),
				})
			}
			if len(videoSplits) > 1 {
				w.mediaArgs = append(w.mediaArgs, hub.MediaSpec{
					MediaType: hub.Video,
					ClockRate: args.ClockRate,
					CodecType: hub.CodecType(strings.ToLower(videoSplits[1])),
				})
			}
			if len(w.mediaArgs) == w.expectedTrackCount {
				w.hub.Notify(ctx, w)
				w.notifiedSource = true
				return nil
			}
		}
	}
}

func (w *WebRTCHandler) OnICEConnectionStateChange(connectionState webrtc.ICEConnectionState, trackArgCh <-chan TrackArgs) {
	ctx := context.Background()
	switch connectionState {
	case webrtc.ICEConnectionStateConnected:
		log.Info(ctx, "ICE Connection State Connected")
		go func() {
			err := w.WaitTrackArgs(ctx, 3*time.Second, trackArgCh)
			if err != nil {
				log.Error(ctx, err, "failed to wait track args")
				return
			}
		}()
	case webrtc.ICEConnectionStateDisconnected:
		w.OnClose(ctx)
		//delete(w.tracks, streamKey)
		log.Info(ctx, "ICE Connection State Disconnected")
	case webrtc.ICEConnectionStateFailed:
		log.Info(ctx, "ICE Connection State Failed")
		_ = w.pc.Close()
	}
}

type TrackArgs struct {
	MimeType  string
	ClockRate uint32
	Channels  uint16
}

func (w *WebRTCHandler) OnTrack(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver, trackArgCh chan<- TrackArgs) {
	ctx := context.Background()
	fmt.Printf("Track has started, of type %s %s\n", track.Kind(), track.Codec().MimeType)
	var videoPackets []*rtp.Packet
	var audioPackets []*rtp.Packet
	var videoPacketsQueue [][]*rtp.Packet
	var audioPacketsQueue [][]*rtp.Packet
	currentVideoTimestamp := uint32(0)
	currentAudioTimestamp := uint32(0)
	trackArgCh <- TrackArgs{
		MimeType:  track.Codec().MimeType,
		ClockRate: track.Codec().ClockRate,
		Channels:  track.Codec().Channels,
	}
	for {
		pkt, _, err := track.ReadRTP()
		if err != nil {
			log.Error(ctx, err, "failed to read rtp")
			break
		}

		switch track.Kind() {
		case webrtc.RTPCodecTypeVideo:
			if len(videoPackets) > 0 && currentVideoTimestamp != pkt.Timestamp {
				videoPacketsQueue = append(videoPacketsQueue, videoPackets)
				videoPackets = nil
			}

			videoPackets = append(videoPackets, pkt)
			currentVideoTimestamp = pkt.Timestamp
			if pkt.Marker {
				videoPacketsQueue = append(videoPacketsQueue, videoPackets)
				videoPackets = nil
			}
		case webrtc.RTPCodecTypeAudio:
			if len(audioPackets) > 0 && currentAudioTimestamp != pkt.Timestamp {
				audioPacketsQueue = append(audioPacketsQueue, audioPackets)
				audioPackets = nil
			}
			audioPackets = append(audioPackets, pkt)
			currentAudioTimestamp = pkt.Timestamp
			if pkt.Marker {
				audioPacketsQueue = append(audioPacketsQueue, audioPackets)
				audioPackets = nil
			}
		}
		if len(videoPacketsQueue) > 0 || len(audioPacketsQueue) > 0 {
			if !w.notifiedSource {
				log.Warn(ctx, "not yet notified source")
			}
		}
		if w.notifiedSource {
			for _, videoPackets := range videoPacketsQueue {
				w.onVideo(ctx, videoPackets)
			}
			videoPacketsQueue = nil
			for _, audioPackets := range audioPacketsQueue {
				w.onAudio(ctx, track.Codec().ClockRate, audioPackets)
			}
			audioPacketsQueue = nil
		}
	}

}
func (w *WebRTCHandler) OnClose(ctx context.Context) error {
	w.hub.Unpublish(w.streamID)
	log.Info(ctx, "OnClose")
	return nil
}

func (w *WebRTCHandler) onVideo(ctx context.Context, packets []*rtp.Packet) error {
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
	pts := w.videoTimestampGen.Generate(int64(packets[0].Timestamp))
	sliceTypes := ingress.SliceTypes(payload)
	w.hub.Publish(w.streamID, &hub.FrameData{
		H264Video: &hub.H264Video{
			PTS:            pts,
			DTS:            pts,
			VideoClockRate: 90000,
			Data:           payload,
			SPS:            nil,
			PPS:            nil,
			SliceTypes:     sliceTypes,
			CodecData:      nil,
		},
		AACAudio: nil,
	})

	return nil
}

func (w *WebRTCHandler) onAudio(ctx context.Context, clockRate uint32, packets []*rtp.Packet) error {
	var opusRTPParser = &codecs.OpusPacket{}
	payload := make([]byte, 0)
	for _, pkt := range packets {
		if len(pkt.Payload) == 0 {
			continue
		}
		b, err := opusRTPParser.Unmarshal(pkt.Payload)
		if err != nil {
			log.Error(ctx, err, "failed to unmarshal opus")
		}
		payload = append(payload, b...)
	}

	if len(payload) == 0 {
		return nil
	}
	pts := w.audioTimestampGen.Generate(int64(packets[0].Timestamp))
	w.hub.Publish(w.streamID, &hub.FrameData{
		OPUSAudio: &hub.OPUSAudio{
			PTS:            pts,
			DTS:            pts,
			AudioClockRate: clockRate,
			Data:           payload,
		},
	})
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

	// Create a MediaEngine object to configure the supported codec
	m := &webrtc.MediaEngine{}
	err = registerCodec(m)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	se := webrtc.SettingEngine{}
	se.SetEphemeralUDPPortRange(30000, 30500)
	if r.dockerMode {
		se.SetNAT1To1IPs([]string{"127.0.0.1"}, webrtc.ICECandidateTypeHost)
	}
	// Create a new RTCPeerConnection
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithSettingEngine(se))
	peerConnection, err := api.NewPeerConnection(peerConnectionConfiguration)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	var rtpSenders []*webrtc.RTPSender
	fmt.Println("tracks: ", len(r.tracks))
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
				_, _, rtcpErr := rtpSender.Read(rtcpBuf)
				if rtcpErr != nil {
					return
				}
			}
		}
	}()
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("ICE Connection State has changed: %s\n", connectionState.String())

		if connectionState == webrtc.ICEConnectionStateFailed {
			delete(r.tracks, streamKey)
			_ = peerConnection.Close()
		}
	})
	// Send answer via HTTP Response
	return writeAnswer3(c, peerConnection, offer, "/whep")
}

func registerCodec(m *webrtc.MediaEngine) error {
	// Setup the codecs you want to use.
	var err error
	if err = m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, Channels: 0, SDPFmtpLine: "", RTCPFeedback: nil},
		PayloadType:        96,
	}, webrtc.RTPCodecTypeVideo); err != nil {
		return err
	}
	if err = m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2, SDPFmtpLine: "", RTCPFeedback: nil},
		PayloadType:        111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return err
	}
	return nil
}
