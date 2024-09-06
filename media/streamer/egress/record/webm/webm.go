package webm

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"liveflow/log"
	"liveflow/media/streamer/egress/record"
	"math"
	"math/rand"
	"os"

	"github.com/at-wat/ebml-go"
	"github.com/at-wat/ebml-go/mkv"
	"github.com/at-wat/ebml-go/mkvcore"
	"github.com/at-wat/ebml-go/webm"
)

var errInitFailed = errors.New("init failed")

const (
	webmAudioTrackNumber = 1
	webmVideoTrackNumber = 2
	trackNameVideo       = "Video"
	trackNameAudio       = "Audio"
	defaultDuration      = 600_000
	defaultTimecode      = 1_000_000
)

const (
	codecIDVP8  = "V_VP8"
	codecIDH264 = "V_MPEG4/ISO/AVC"
	codecIDOPUS = "A_OPUS"
	codecIDAAC  = "A_AAC"
)

type Name string

const (
	ContainerWebM Name = "webm"
	ContainerMKV  Name = "mkv"
)

type EBMLMuxer struct {
	writers          []mkvcore.BlockWriteCloser
	container        Name
	tempFileName     string
	audioSampleRate  float64
	audioChannels    uint64
	durationPos      uint64
	duration         int64
	audioStreamIndex int
	videoStreamIndex int
}

func NewEBMLMuxer(sampleRate int, channels int, container Name) *EBMLMuxer {
	return &EBMLMuxer{
		writers:         nil,
		tempFileName:    "",
		audioSampleRate: float64(sampleRate),
		audioChannels:   uint64(channels),
		durationPos:     0,
		duration:        0,
		container:       container,
	}
}

func (w *EBMLMuxer) makeWebmWriters() ([]mkvcore.BlockWriteCloser, error) {
	const (
		trackTypeVideo = 1
		trackTypeAudio = 2
	)
	tempFile, err := record.CreateFileInDir(fmt.Sprintf("videos/%d.webm", rand.Int()))
	if err != nil {
		return nil, err
	}
	w.audioStreamIndex = 0
	w.videoStreamIndex = 1
	trackEntries := []webm.TrackEntry{
		{
			Name:        trackNameAudio,
			TrackNumber: webmAudioTrackNumber,
			TrackUID:    webmAudioTrackNumber,
			CodecID:     codecIDOPUS,
			TrackType:   trackTypeAudio,
			Audio: &webm.Audio{
				SamplingFrequency: w.audioSampleRate,
				Channels:          w.audioChannels,
			},
		},
		{
			Name:        trackNameVideo,
			TrackNumber: webmVideoTrackNumber,
			TrackUID:    webmVideoTrackNumber,
			CodecID:     codecIDVP8,
			TrackType:   trackTypeVideo,
			Video: &webm.Video{
				PixelWidth:  1280,
				PixelHeight: 720,
			},
		},
	}
	writers, err := webm.NewSimpleBlockWriter(tempFile, trackEntries,
		mkvcore.WithSeekHead(true),
		mkvcore.WithOnErrorHandler(func(err error) {
			log.Error(context.Background(), err, "failed to construct webm writer (error)")
		}),
		mkvcore.WithOnFatalHandler(func(err error) {
			log.Error(context.Background(), err, "failed to construct webm writer (fatal)")
		}),
		mkvcore.WithSegmentInfo(&webm.Info{
			TimecodeScale: defaultTimecode, // 1ms
			MuxingApp:     "mrw-v4.ebml-go.webm",
			WritingApp:    "mrw-v4.ebml-go.webm",
			Duration:      defaultDuration, // Arbitrarily set to default videoSplitIntervalMs, final value is adjusted in writeTrailer.
		}),
		mkvcore.WithMarshalOptions(ebml.WithElementWriteHooks(func(e *ebml.Element) {
			switch e.Name {
			case "Duration":
				w.durationPos = e.Position + 4 // Duration header size = 3, SegmentInfo header size delta = 1
			}
		})),
	)
	if err != nil {
		return nil, err
	}
	w.tempFileName = tempFile.Name()
	var mkvWriters []mkvcore.BlockWriteCloser
	for _, writer := range writers {
		mkvWriters = append(mkvWriters, writer)
	}
	return mkvWriters, nil
}

func (w *EBMLMuxer) makeMKVWriters() ([]mkvcore.BlockWriteCloser, error) {
	const (
		trackTypeVideo = 1
		trackTypeAudio = 2
	)
	tempFile, err := record.CreateFileInDir(fmt.Sprintf("videos/%d.mkv", rand.Int()))
	if err != nil {
		return nil, err
	}
	var mkvTrackDesc []mkvcore.TrackDescription
	w.audioStreamIndex = 0
	w.videoStreamIndex = 1
	mkvTrackDesc = append(mkvTrackDesc, mkvcore.TrackDescription{
		TrackNumber: 1,
		TrackEntry: webm.TrackEntry{
			Name:        trackNameAudio,
			TrackNumber: 1,
			TrackUID:    1,
			CodecID:     codecIDOPUS,
			TrackType:   trackTypeAudio,
			Audio: &webm.Audio{
				SamplingFrequency: w.audioSampleRate,
				Channels:          2,
			},
		},
	})
	mkvTrackDesc = append(mkvTrackDesc, mkvcore.TrackDescription{
		TrackNumber: webmVideoTrackNumber,
		TrackEntry: webm.TrackEntry{
			TrackNumber:     webmVideoTrackNumber,
			TrackUID:        webmVideoTrackNumber,
			TrackType:       trackTypeVideo,
			DefaultDuration: 0,
			Name:            trackNameVideo,
			CodecID:         codecIDH264,
			SeekPreRoll:     0,
			// TODO: The resolution may need to be written later, but it works fine without it for now.
			//Video: &webm.Video{
			//	PixelWidth:  1280,
			//	PixelHeight: 720,
			//},
		},
	})
	var mkvWriters []mkvcore.BlockWriteCloser
	mkvWriters, err = mkvcore.NewSimpleBlockWriter(tempFile, mkvTrackDesc,
		mkvcore.WithSeekHead(true),
		mkvcore.WithEBMLHeader(mkv.DefaultEBMLHeader),
		mkvcore.WithSegmentInfo(&webm.Info{
			TimecodeScale: defaultTimecode, // 1ms
			MuxingApp:     "mrw-v4.ebml-go.mkv",
			WritingApp:    "mrw-v4.ebml-go.mkv",
			Duration:      defaultDuration, // Arbitrarily set to default videoSplitIntervalMs, final value is adjusted in writeTrailer.
		}),
		mkvcore.WithBlockInterceptor(webm.DefaultBlockInterceptor),
		mkvcore.WithMarshalOptions(ebml.WithElementWriteHooks(func(e *ebml.Element) {
			switch e.Name {
			case "Duration":
				w.durationPos = e.Position + 4 // Duration header size = 3, SegmentInfo header size delta = 1
			}
		})),
	)
	if err != nil {
		return nil, err
	}
	w.tempFileName = tempFile.Name()
	return mkvWriters, nil
}

func (w *EBMLMuxer) Init(_ context.Context) error {
	var err error
	switch w.container {
	case ContainerWebM:
		w.writers, err = w.makeWebmWriters()
	case ContainerMKV:
		w.writers, err = w.makeMKVWriters()
	default:
		return errInitFailed
	}
	return err
}

func (w *EBMLMuxer) Initialized() bool {
	return len(w.writers) > 0
}

func (w *EBMLMuxer) WriteVideo(data []byte, keyframe bool, pts uint64, _ uint64) error {
	if _, err := w.writers[w.videoStreamIndex].Write(keyframe, int64(pts), data); err != nil {
		return err
	}
	if w.duration < int64(pts) {
		w.duration = int64(pts)
	}
	return nil
}

func (w *EBMLMuxer) WriteAudio(data []byte, keyframe bool, pts uint64, _ uint64) error {
	if _, err := w.writers[w.audioStreamIndex].Write(keyframe, int64(pts), data); err != nil {
		return err
	}
	if w.duration < int64(pts) {
		w.duration = int64(pts)
	}
	return nil
}

func (w *EBMLMuxer) Finalize(ctx context.Context) error {
	defer func() {
		w.cleanup()
	}()
	log.Info(ctx, "finalize webm muxer")
	fileName := w.tempFileName
	for _, writer := range w.writers {
		if err := writer.Close(); err != nil {
			log.Error(ctx, err, "failed to close writer")
		}
	}
	if err := w.overwritePTS(ctx, fileName); err != nil {
		return err
	}
	return nil
}

func (w *EBMLMuxer) ContainerName() string {
	return string(w.container)
}

func (w *EBMLMuxer) overwritePTS(ctx context.Context, fileName string) error {
	tempFile, err := os.OpenFile(fileName, os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if err := tempFile.Close(); err != nil {
			log.Error(ctx, err, "failed to close temp file")
		}
	}()
	ptsBytes, _ := EncodeFloat64(float64(w.duration))
	if _, err := tempFile.WriteAt(ptsBytes, int64(w.durationPos)); err != nil {
		return err
	}
	return nil
}

func (w *EBMLMuxer) cleanup() {
	w.writers = nil
	w.tempFileName = ""
	w.duration = 0
	w.durationPos = 0
}

func EncodeFloat64(i float64) ([]byte, error) {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, math.Float64bits(i))
	return b, nil
}
