package webm

import (
	"context"
	"errors"
	"fmt"
	"liveflow/log"
	"liveflow/media/hub"
	"liveflow/media/streamer/egress/record"
	"liveflow/media/streamer/fields"
	"liveflow/media/streamer/processes"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/deepch/vdk/codec/aacparser"
	"github.com/sirupsen/logrus"
)

var (
	ErrUnsupportedCodec = errors.New("unsupported codec")
)

const (
	audioSampleRate = 48000
)

type WebMArgs struct {
	Hub             *hub.Hub
	SplitIntervalMS int64  // Add SplitIntervalMS to arguments
	StreamID        string // Add StreamID
}

type WebM struct {
	hub                     *hub.Hub
	webmMuxer               *EBMLMuxer
	samples                 int
	splitIntervalMS         int64
	lastSplitTime           int64
	splitPending            bool
	streamID                string
	audioTranscodingProcess *processes.AudioTranscodingProcess
	mediaSpecs              []hub.MediaSpec
}

func NewWEBM(args WebMArgs) *WebM {
	return &WebM{
		hub:             args.Hub,
		splitIntervalMS: args.SplitIntervalMS,
		streamID:        args.StreamID,
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
	w.mediaSpecs = source.MediaSpecs()

	ctx = log.WithFields(ctx, logrus.Fields{
		fields.StreamID:   source.StreamID(),
		fields.SourceName: source.Name(),
	})
	log.Info(ctx, "start webm")
	sub := w.hub.Subscribe(source.StreamID())
	go func() {
		// Initialize splitting logic
		err := w.createNewMuxer(ctx, int(audioClockRate))
		if err != nil {
			log.Error(ctx, err, "failed to create webm muxer")
			return
		}

		// Initialize audio transcoding process if needed
		if hub.HasCodecType(source.MediaSpecs(), hub.CodecTypeAAC) {
			w.audioTranscodingProcess = processes.NewTranscodingProcess(astiav.CodecIDAac, astiav.CodecIDOpus, audioSampleRate)
			w.audioTranscodingProcess.Init()
			defer w.audioTranscodingProcess.Close()
		}

		for data := range sub {
			// Check if we need to initiate a split

			if data.H264Video != nil {
				if !w.splitPending && data.H264Video.RawDTS()-w.lastSplitTime >= w.splitIntervalMS {
					w.splitPending = true
				}
				w.onVideo(ctx, data.H264Video)
			}
			if data.AACAudio != nil {
				w.onAACAudio(ctx, data.AACAudio)
			} else if data.OPUSAudio != nil {
				w.onAudio(ctx, data.OPUSAudio)
			}
		}
		// Ensure the muxer is finalized
		w.closeMuxer(ctx)
	}()
	return nil
}

// createNewMuxer initializes a new EBMLMuxer
func (w *WebM) createNewMuxer(ctx context.Context, audioClockRate int) error {
	// Initialize new muxer
	w.webmMuxer = NewEBMLMuxer(audioClockRate, 2, ContainerMKV)
	err := w.webmMuxer.Init(ctx)
	if err != nil {
		return err
	}
	return nil
}

// closeMuxer finalizes the current muxer and writes to the output file
func (w *WebM) closeMuxer(ctx context.Context) {
	if w.webmMuxer != nil {
		// Create output file with timestamp
		timestamp := time.Now().Format("2006-01-02-15-04-05")
		fileName := fmt.Sprintf("videos/%s_%s.mkv", w.streamID, timestamp)
		outputFile, err := record.CreateFileInDir(fileName)
		if err != nil {
			log.Error(ctx, err, "failed to create output file")
			return
		}
		defer outputFile.Close()

		// Finalize muxer with output file
		err = w.webmMuxer.Finalize(ctx, outputFile)
		if err != nil {
			log.Error(ctx, err, "failed to finalize muxer")
		}
		w.webmMuxer = nil
	}
}

// splitMuxer handles the logic to split the WebM file
func (w *WebM) splitMuxer(ctx context.Context) error {
	// Close current muxer
	w.closeMuxer(ctx)
	// Create a new muxer
	audioClockRate, err := hub.AudioClockRate(w.mediaSpecs)
	if err != nil {
		return err
	}
	return w.createNewMuxer(ctx, int(audioClockRate))
}

func (w *WebM) onVideo(ctx context.Context, data *hub.H264Video) {
	keyFrame := false
	for _, sliceType := range data.SliceTypes {
		if sliceType == hub.SliceI {
			keyFrame = true
			break
		}
	}

	// If a split is pending and we have a keyframe, perform the split
	if w.splitPending && keyFrame {
		err := w.splitMuxer(ctx)
		if err != nil {
			log.Error(ctx, err, "failed to split webm file")
			return
		}
		w.lastSplitTime = data.RawDTS()
		w.splitPending = false // Reset the split pending flag
	}

	err := w.webmMuxer.WriteVideo(data.Data, keyFrame, uint64(data.RawPTS()-w.lastSplitTime), uint64(data.RawDTS()-w.lastSplitTime))
	if err != nil {
		log.Error(ctx, err, "failed to write video")
	}
}

func (w *WebM) onAudio(ctx context.Context, data *hub.OPUSAudio) {
	fmt.Println("dts: ", data.RawDTS())
	err := w.webmMuxer.WriteAudio(data.Data, false, uint64(data.RawPTS()-w.lastSplitTime), uint64(data.RawDTS()-w.lastSplitTime))
	if err != nil {
		log.Error(ctx, err, "failed to write audio")
	}
}

func (w *WebM) onAACAudio(ctx context.Context, aac *hub.AACAudio) {
	if len(aac.Data) == 0 {
		log.Warn(ctx, "no data")
		return
	}
	if aac.MPEG4AudioConfig == nil {
		log.Warn(ctx, "no config")
		return
	}
	const (
		aacSamples     = 1024
		adtsHeaderSize = 7
	)
	adtsHeader := make([]byte, adtsHeaderSize)

	aacparser.FillADTSHeader(adtsHeader, *aac.MPEG4AudioConfig, aacSamples, len(aac.Data))
	audioDataWithADTS := append(adtsHeader, aac.Data...)
	packets, err := w.audioTranscodingProcess.Process(&processes.MediaPacket{
		Data: audioDataWithADTS,
		PTS:  aac.PTS,
		DTS:  aac.DTS,
	})
	if err != nil {
		fmt.Println(err)
	}
	for _, packet := range packets {
		w.onAudio(ctx, &hub.OPUSAudio{
			Data:           packet.Data,
			PTS:            packet.PTS,
			DTS:            packet.DTS,
			AudioClockRate: uint32(packet.SampleRate),
		})
	}
}
