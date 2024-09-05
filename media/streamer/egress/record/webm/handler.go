package webm

import (
	"context"
	"errors"
	"fmt"
	astiav "liveflow/goastiav"
	"liveflow/log"
	"liveflow/media/hub"
	"liveflow/media/streamer/fields"
	"liveflow/media/streamer/processes"

	"github.com/deepch/vdk/codec/aacparser"
	"github.com/pion/webrtc/v3"
	"github.com/sirupsen/logrus"
)

var (
	ErrNotContainAudioOrVideo = errors.New("media spec does not contain audio or video")
	ErrUnsupportedCodec       = errors.New("unsupported codec")
)

type WebMArgs struct {
	Tracks map[string][]*webrtc.TrackLocalStaticRTP
	Hub    *hub.Hub
}

// whip
type WebM struct {
	hub       *hub.Hub
	webmMuxer *WebmMuxer
	samples   int
}

func NewWEBM(args WebMArgs) *WebM {
	return &WebM{
		hub: args.Hub,
	}
}

func (w *WebM) Start(ctx context.Context, source hub.Source) error {
	if !hub.HasCodecType(source.MediaSpecs(), hub.CodecTypeAAC) && !hub.HasCodecType(source.MediaSpecs(), hub.CodecTypeOpus) {
		return ErrUnsupportedCodec
	}
	if !hub.HasCodecType(source.MediaSpecs(), hub.CodecTypeH264) {
		return ErrUnsupportedCodec
	}
	audioClockRate, err := hub.AudioClockRate(source.MediaSpecs())
	if err != nil {
		return err
	}

	ctx = log.WithFields(ctx, logrus.Fields{
		fields.StreamID:   source.StreamID(),
		fields.SourceName: source.Name(),
	})
	muxer := NewEBMLMuxer(int(audioClockRate), 2, ContainerMKV)
	err = muxer.Init(ctx)
	if err != nil {
		return err
	}
	log.Info(ctx, "start webm")
	sub := w.hub.Subscribe(source.StreamID())
	aProcess := processes.NewTranscodingProcess(astiav.CodecIDAac, astiav.CodecIDOpus)
	aProcess.Init()
	go func() {
		for data := range sub {
			if data.H264Video != nil {
				w.OnVideo(ctx, muxer, data.H264Video)
			}
			if data.AACAudio != nil {
				w.OnAACAudio(ctx, muxer, data.AACAudio, aProcess)
			} else if data.OPUSAudio != nil {
				w.OnAudio(ctx, muxer, data.OPUSAudio)
			}
		}
		err = muxer.Finalize(ctx)
		if err != nil {
			panic(err)
		}
	}()
	return nil
}

func (w *WebM) OnVideo(ctx context.Context, muxer *WebmMuxer, data *hub.H264Video) {
	keyFrame := data.SliceType == hub.SliceI
	err := muxer.WriteVideo(data.Data, keyFrame, uint64(data.RawPTS()), uint64(data.RawDTS()))
	if err != nil {
		log.Error(ctx, err, "failed to write video")
	}
}

func (w *WebM) OnAudio(ctx context.Context, muxer *WebmMuxer, data *hub.OPUSAudio) {
	err := muxer.WriteAudio(data.Data, false, uint64(data.RawPTS()), uint64(data.RawDTS()))
	if err != nil {
		log.Error(ctx, err, "failed to write audio")
	}
}

func (w *WebM) OnAACAudio(ctx context.Context, muxer *WebmMuxer, aac *hub.AACAudio, transcodingProcess *processes.AudioTranscodingProcess) {
	if len(aac.Data) == 0 {
		fmt.Println("no data")
		return
	}
	if aac.MPEG4AudioConfig == nil {
		fmt.Println("no config")
		return
	}
	const (
		aacSamples     = 1024
		adtsHeaderSize = 7
	)
	adtsHeader := make([]byte, adtsHeaderSize)

	aacparser.FillADTSHeader(adtsHeader, *aac.MPEG4AudioConfig, aacSamples, len(aac.Data))
	audioDataWithADTS := append(adtsHeader, aac.Data...)
	packets, err := transcodingProcess.Process(&processes.MediaPacket{
		Data: audioDataWithADTS,
		PTS:  aac.PTS,
		DTS:  aac.DTS,
	})
	if err != nil {
		fmt.Println(err)
	}
	for _, packet := range packets {
		w.OnAudio(ctx, muxer, &hub.OPUSAudio{
			Data:           packet.Data,
			PTS:            packet.PTS,
			DTS:            packet.DTS,
			AudioClockRate: audioSampleRate,
		})
	}
}
