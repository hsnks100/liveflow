package hls

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bluenviron/gohlslib"
	"github.com/bluenviron/gohlslib/pkg/codecs"

	"github.com/deepch/vdk/codec/h264parser"

	"mrw-clone/log"
	"mrw-clone/media/hlshub"
	"mrw-clone/media/hub"
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

func (h *HLS) Start(ctx context.Context, streamID string) {
	fmt.Println("@@@ Start StreamID: ", streamID)
	sub := h.hub.Subscribe(streamID)
	go func() {
		for data := range sub {
			if data.AACAudio != nil {
				if len(data.AACAudio.MPEG4AudioConfigBytes) > 0 {
					muxer, err := h.makeMuxer(data.AACAudio.MPEG4AudioConfigBytes)
					if err != nil {
						log.Error(ctx, err)
					}
					h.hlsHub.StoreMuxer(streamID, "pass", muxer)
					err = muxer.Start()
					if err != nil {
						log.Error(ctx, err)
					}
					h.muxer = muxer
				}
				if h.muxer != nil {
					//fmt.Println("audio time: ", time.Now(), "PTS: ", data.AACAudio.RawDTS())
					h.muxer.WriteMPEG4Audio(time.Now(), time.Duration(data.AACAudio.DTS)*time.Millisecond, [][]byte{data.AACAudio.Data})
				}
			}
			if data.H264Video != nil {
				if h.muxer != nil {
					//fmt.Println("video time: ", time.Now(), "PTS: ", data.H264Video.RawDTS())
					au, _ := h264parser.SplitNALUs(data.H264Video.Data)
					err := h.muxer.WriteH264(time.Now(), time.Duration(data.H264Video.DTS)*time.Millisecond, au)
					if err != nil {
						log.Errorf(ctx, "failed to write h264: %v", err)
					}
				}
			}
		}
		fmt.Println("@@@ [HLS] end of streamID: ", streamID)
	}()
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
