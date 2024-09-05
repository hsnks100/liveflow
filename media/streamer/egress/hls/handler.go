package hls

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bluenviron/gohlslib"
	"github.com/bluenviron/gohlslib/pkg/codecs"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/sirupsen/logrus"

	"liveflow/log"
	"liveflow/media/hlshub"
	"liveflow/media/hub"
	"liveflow/media/streamer/fields"
)

var (
	ErrNotContainAudioOrVideo = errors.New("media spec does not contain audio or video")
	ErrUnsupportedCodec       = errors.New("unsupported codec")
)

type HLS struct {
	hub    *hub.Hub
	hlsHub *hlshub.HLSHub
	muxer  *gohlslib.Muxer
}

type HLSArgs struct {
	Hub    *hub.Hub
	HLSHub *hlshub.HLSHub
}

func NewHLS(args HLSArgs) *HLS {
	return &HLS{
		hub:    args.Hub,
		hlsHub: args.HLSHub,
	}
}

func (h *HLS) Start(ctx context.Context, source hub.Source) error {
	if !hub.HasCodecType(source.MediaSpecs(), hub.CodecTypeAAC) {
		return ErrUnsupportedCodec
	}
	if !hub.HasCodecType(source.MediaSpecs(), hub.CodecTypeH264) {
		return ErrUnsupportedCodec
	}
	ctx = log.WithFields(ctx, logrus.Fields{
		fields.StreamID:   source.StreamID(),
		fields.SourceName: source.Name(),
	})
	log.Info(ctx, "start hls")
	log.Info(ctx, "view url: ", "http://localhost:8044/hls/"+source.StreamID()+"/master.m3u8")
	sub := h.hub.Subscribe(source.StreamID())
	go func() {
		for data := range sub {
			if data.AACAudio != nil {
				h.OnAudio(ctx, source, data.AACAudio)
			}
			if data.H264Video != nil {
				h.OnVideo(ctx, data.H264Video)
			}
		}
		fmt.Println("@@@ [HLS] end of streamID: ", source.StreamID())
	}()
	return nil
}

func (h *HLS) OnAudio(ctx context.Context, source hub.Source, aacAudio *hub.AACAudio) {
	if len(aacAudio.MPEG4AudioConfigBytes) > 0 {
		if h.muxer == nil {
			muxer, err := h.makeMuxer(aacAudio.MPEG4AudioConfigBytes)
			if err != nil {
				log.Error(ctx, err)
			}
			h.hlsHub.StoreMuxer(source.StreamID(), "pass", muxer)
			err = muxer.Start()
			if err != nil {
				log.Error(ctx, err)
			}
			h.muxer = muxer
		}
	}
	if h.muxer != nil {
		audioData := make([]byte, len(aacAudio.Data))
		copy(audioData, aacAudio.Data)
		h.muxer.WriteMPEG4Audio(time.Now(), time.Duration(aacAudio.RawDTS())*time.Millisecond, [][]byte{audioData})
	}
}

func (h *HLS) OnVideo(ctx context.Context, h264Video *hub.H264Video) {
	if h.muxer != nil {
		au, _ := h264parser.SplitNALUs(h264Video.Data)
		err := h.muxer.WriteH264(time.Now(), time.Duration(h264Video.RawDTS())*time.Millisecond, au)
		if err != nil {
			log.Errorf(ctx, "failed to write h264: %v", err)
		}
	}
}

func (h *HLS) makeMuxer(extraData []byte) (*gohlslib.Muxer, error) {
	var audioTrack *gohlslib.Track
	if len(extraData) > 0 {
		mpeg4Audio := &codecs.MPEG4Audio{}
		err := mpeg4Audio.Unmarshal(extraData)
		if err != nil {
			return nil, errors.New("failed to unmarshal mpeg4 audio")
		}
		audioTrack = &gohlslib.Track{
			Codec: mpeg4Audio,
		}
	}
	muxer := &gohlslib.Muxer{
		VideoTrack: &gohlslib.Track{
			Codec: &codecs.H264{},
		},
		AudioTrack: audioTrack,
	}
	llHLS := false
	if llHLS {
		muxer.Variant = gohlslib.MuxerVariantLowLatency
		muxer.PartDuration = 500 * time.Millisecond
	} else {
		muxer.Variant = gohlslib.MuxerVariantMPEGTS
		muxer.SegmentDuration = 1 * time.Second
	}
	return muxer, nil
}
