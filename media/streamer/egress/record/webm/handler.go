package webm

import (
	"context"
	"errors"
	"fmt"
	"liveflow/log"
	"liveflow/media/hub"
	"liveflow/media/streamer/fields"

	"github.com/pion/webrtc/v3"
	"github.com/sirupsen/logrus"
)

var (
	ErrNotContainAudioOrVideo = errors.New("media spec does not contain audio or video")
	ErrUnsupportedCodec       = errors.New("unsupported codec")
)

type WEBMArgs struct {
	Tracks map[string][]*webrtc.TrackLocalStaticRTP
	Hub    *hub.Hub
}

// whip
type WEBM struct {
	hub *hub.Hub
}

func NewWEBM(args WEBMArgs) *WEBM {
	return &WEBM{
		hub: args.Hub,
	}
}

func (w *WEBM) Start(ctx context.Context, source hub.Source) error {
	containsAudio := false
	containsVideo := false
	audioCodec := ""
	videoCodec := ""
	audioClockRate := 0
	videoClockRate := 0
	_ = videoClockRate
	for _, spec := range source.MediaSpecs() {
		if spec.MediaType == hub.Audio {
			containsAudio = true
			audioCodec = spec.CodecType
			audioClockRate = int(spec.ClockRate)
		}
		if spec.MediaType == hub.Video {
			containsVideo = true
			videoCodec = spec.CodecType
			videoClockRate = int(spec.ClockRate)
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
	muxer := NewWebmMuxer(audioClockRate, 2, MKV)
	err := muxer.Init(ctx)
	if err != nil {
		return err
	}
	log.Info(ctx, "start whep")
	sub := w.hub.Subscribe(source.StreamID())
	go func() {
		for data := range sub {
			if data.H264Video != nil {
				keyFrame := data.H264Video.SliceType == hub.SliceI
				err := muxer.WriteVideo(data.H264Video.Data, keyFrame, uint64(data.H264Video.RawPTS()), uint64(data.H264Video.RawDTS()))
				if err != nil {
					log.Error(ctx, err, "failed to write video")
				}
			}
			if data.OPUSAudio != nil {
				err := muxer.WriteAudio(data.OPUSAudio.Data, false, uint64(data.OPUSAudio.RawPTS()), uint64(data.OPUSAudio.RawDTS()))
				if err != nil {
					log.Error(ctx, err, "failed to write audio")
				}
			}
		}
		err = muxer.Finalize(ctx)
		if err != nil {
			panic(err)
		}
	}()
	return nil
}
