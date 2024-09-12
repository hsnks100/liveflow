package hls

import (
	"context"
	"errors"
	"fmt"
	"liveflow/media/streamer/processes"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/bluenviron/gohlslib"
	"github.com/bluenviron/gohlslib/pkg/codecs"
	"github.com/deepch/vdk/codec/aacparser"
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

const (
	audioSampleRate = 48000
)

type HLS struct {
	hub                   *hub.Hub
	hlsHub                *hlshub.HLSHub
	port                  int
	muxer                 *gohlslib.Muxer
	mpeg4AudioConfigBytes []byte
	mpeg4AudioConfig      *aacparser.MPEG4AudioConfig
	llHLS                 bool
	diskRam               bool
}

type HLSArgs struct {
	Hub     *hub.Hub
	HLSHub  *hlshub.HLSHub
	Port    int
	LLHLS   bool
	DiskRam bool
}

func NewHLS(args HLSArgs) *HLS {
	return &HLS{
		hub:     args.Hub,
		hlsHub:  args.HLSHub,
		port:    args.Port,
		llHLS:   args.LLHLS,
		diskRam: args.DiskRam,
	}
}

func (h *HLS) Start(ctx context.Context, source hub.Source) error {
	if !hub.HasCodecType(source.MediaSpecs(), hub.CodecTypeAAC) && !hub.HasCodecType(source.MediaSpecs(), hub.CodecTypeOpus) {
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
	log.Info(ctx, "view url: ",
		fmt.Sprintf("http://localhost:8044/m3u8player.html?streamid=%s", source.StreamID()))

	sub := h.hub.Subscribe(source.StreamID())
	go func() {
		var audioTranscodingProcess *processes.AudioTranscodingProcess
		for data := range sub {
			if data.OPUSAudio != nil {
				if audioTranscodingProcess == nil {
					audioTranscodingProcess = processes.NewTranscodingProcess(astiav.CodecIDOpus, astiav.CodecIDAac, audioSampleRate)
					audioTranscodingProcess.Init()
					defer audioTranscodingProcess.Close()
					h.mpeg4AudioConfigBytes = audioTranscodingProcess.ExtraData()
					tmpAudioCodec, err := aacparser.NewCodecDataFromMPEG4AudioConfigBytes(h.mpeg4AudioConfigBytes)
					if err != nil {
						log.Error(ctx, err)
					}
					h.mpeg4AudioConfig = &tmpAudioCodec.Config
				}
				h.onOPUSAudio(ctx, source, audioTranscodingProcess, data.OPUSAudio)
			} else {
				if data.AACAudio != nil {
					h.onAudio(ctx, source, data.AACAudio)
				}
			}
			if data.H264Video != nil {
				h.onVideo(ctx, data.H264Video)
			}
		}
		log.Info(ctx, "[HLS] end of streamID: ", source.StreamID())
	}()
	return nil
}

func (h *HLS) onAudio(ctx context.Context, source hub.Source, aacAudio *hub.AACAudio) {
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

func (h *HLS) onVideo(ctx context.Context, h264Video *hub.H264Video) {
	if h.muxer != nil {
		au, _ := h264parser.SplitNALUs(h264Video.Data)
		err := h.muxer.WriteH264(time.Now(), time.Duration(h264Video.RawDTS())*time.Millisecond, au)
		if err != nil {
			log.Errorf(ctx, "failed to write h264: %v", err)
		}
	}
}

func (h *HLS) onOPUSAudio(ctx context.Context, source hub.Source, audioTranscodingProcess *processes.AudioTranscodingProcess, opusAudio *hub.OPUSAudio) {
	packets, err := audioTranscodingProcess.Process(&processes.MediaPacket{
		Data: opusAudio.Data,
		PTS:  opusAudio.PTS,
		DTS:  opusAudio.DTS,
	})
	if err != nil {
		fmt.Println(err)
	}
	for _, packet := range packets {
		h.onAudio(ctx, source, &hub.AACAudio{
			Data:                  packet.Data,
			SequenceHeader:        false,
			MPEG4AudioConfigBytes: h.mpeg4AudioConfigBytes,
			MPEG4AudioConfig:      h.mpeg4AudioConfig,
			PTS:                   packet.PTS,
			DTS:                   packet.DTS,
			AudioClockRate:        uint32(packet.SampleRate),
		})
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
	var directory string
	if h.diskRam {
		directory = "/tmp"
	}
	muxer := &gohlslib.Muxer{
		VideoTrack: &gohlslib.Track{
			Codec: &codecs.H264{},
		},
		AudioTrack: audioTrack,
		Directory:  directory,
	}

	if h.llHLS {
		muxer.Variant = gohlslib.MuxerVariantLowLatency
		muxer.PartDuration = 500 * time.Millisecond
	} else {
		muxer.Variant = gohlslib.MuxerVariantMPEGTS
		muxer.SegmentDuration = 1 * time.Second
	}
	return muxer, nil
}
