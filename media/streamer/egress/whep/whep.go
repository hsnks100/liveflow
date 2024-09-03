package whep

import (
	"context"
	"errors"
	"fmt"

	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v3"
	"github.com/sirupsen/logrus"

	"liveflow/log"
	"liveflow/media/hub"
	"liveflow/media/streamer/fields"
)

var (
	ErrNotContainAudioOrVideo = errors.New("media spec does not contain audio or video")
	ErrUnsupportedCodec       = errors.New("unsupported codec")
)

type WHEPArgs struct {
	Tracks map[string][]*webrtc.TrackLocalStaticRTP
	Hub    *hub.Hub
}

// whip
type WHEP struct {
	hub                *hub.Hub
	tracks             map[string][]*webrtc.TrackLocalStaticRTP
	audioTrack         *webrtc.TrackLocalStaticRTP
	videoTrack         *webrtc.TrackLocalStaticRTP
	audioPacketizer    rtp.Packetizer
	videoPacketizer    rtp.Packetizer
	lastAudioTimestamp int64
	lastVideoTimestamp int64
}

func NewWHEP(args WHEPArgs) *WHEP {
	return &WHEP{
		hub:    args.Hub,
		tracks: args.Tracks,
	}
}

func (w *WHEP) Start(ctx context.Context, source hub.Source) error {
	containsAudio := false
	containsVideo := false
	audioCodec := ""
	videoCodec := ""
	for _, spec := range source.MediaSpecs() {
		if spec.MediaType == hub.Audio {
			containsAudio = true
			audioCodec = spec.CodecType
		}
		if spec.MediaType == hub.Video {
			containsVideo = true
			videoCodec = spec.CodecType
		}
	}
	if !containsVideo || !containsAudio {
		return ErrNotContainAudioOrVideo
	}
	fmt.Println("audioCodec", audioCodec)
	fmt.Println("videoCodec", videoCodec)
	if audioCodec != "opus" {
		return fmt.Errorf("%w: %s", ErrUnsupportedCodec, audioCodec)
	}
	if videoCodec != "h264" {
		return fmt.Errorf("%w: %s", ErrUnsupportedCodec, videoCodec)
	}
	ctx = log.WithFields(ctx, logrus.Fields{
		fields.StreamID:   source.StreamID(),
		fields.SourceName: source.Name(),
	})
	log.Info(ctx, "start whep")
	sub := w.hub.Subscribe(source.StreamID())
	go func() {
		for data := range sub {
			if data.H264Video != nil {
				if w.videoTrack == nil {
					var err error
					w.videoTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "pion")
					if err != nil {
						panic(err)
					}
					w.tracks[source.StreamID()] = append(w.tracks[source.StreamID()], w.videoTrack)
					ssrc := uint32(110)
					const (
						h264PayloadType = 96
						mtu             = 1400
					)
					w.videoPacketizer = rtp.NewPacketizer(mtu, h264PayloadType, ssrc, &codecs.H264Payloader{}, rtp.NewRandomSequencer(), data.H264Video.VideoClockRate)
				}
				duration := data.H264Video.DTS - w.lastVideoTimestamp
				packets := w.videoPacketizer.Packetize(data.H264Video.Data, uint32(duration))
				for _, p := range packets {
					if err := w.videoTrack.WriteRTP(p); err != nil {
						panic(err)
					}
				}
				w.lastVideoTimestamp = data.H264Video.DTS
			}
			if data.AACAudio != nil {
				fmt.Println("it must transcoding")
			}
			if data.OPUSAudio != nil {
				if w.audioTrack == nil {
					var err error
					w.audioTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion")
					if err != nil {
						panic(err)
					}
					w.tracks[source.StreamID()] = append(w.tracks[source.StreamID()], w.audioTrack)
					ssrc := uint32(111)
					const (
						opusPayloadType = 111
						mtu             = 1400
					)
					w.audioPacketizer = rtp.NewPacketizer(mtu, opusPayloadType, ssrc, &codecs.OpusPayloader{}, rtp.NewRandomSequencer(), data.OPUSAudio.AudioClockRate)
				}
				// TODO: /whep 요청 들어오면 거기서 트랙만들고 센더 만들고 등록해주기
				duration := data.OPUSAudio.DTS - w.lastAudioTimestamp
				packets := w.audioPacketizer.Packetize(data.OPUSAudio.Data, uint32(duration))
				for _, p := range packets {
					if err := w.audioTrack.WriteRTP(p); err != nil {
						panic(err)
					}
				}
				w.lastAudioTimestamp = data.OPUSAudio.DTS
			}
		}
	}()
	return nil
}
