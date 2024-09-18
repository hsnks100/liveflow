package webm

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
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
	tempFile         *os.File
	container        Name
	audioSampleRate  float64
	audioChannels    uint64
	durationPos      int64
	duration         int64
	audioStreamIndex int
	videoStreamIndex int
}

func NewEBMLMuxer(sampleRate int, channels int, container Name) *EBMLMuxer {
	return &EBMLMuxer{
		writers:         nil,
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

	var err error
	w.tempFile, err = ioutil.TempFile("", "ebmlmuxer-*.webm")
	if err != nil {
		return nil, err
	}

	writers, err := webm.NewSimpleBlockWriter(w.tempFile, trackEntries,
		mkvcore.WithSeekHead(true),
		mkvcore.WithSegmentInfo(&webm.Info{
			TimecodeScale: defaultTimecode, // 1ms
			MuxingApp:     "your_app_name",
			WritingApp:    "your_app_name",
			Duration:      defaultDuration, // Placeholder duration; final value is adjusted in overwritePTS.
		}),
		mkvcore.WithMarshalOptions(ebml.WithElementWriteHooks(func(e *ebml.Element) {
			if e.Name == "Duration" {
				w.durationPos = int64(e.Position + 4) // Adjust position to overwrite duration later.
			}
		})),
	)
	if err != nil {
		return nil, err
	}
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
	w.audioStreamIndex = 0
	w.videoStreamIndex = 1

	mkvTrackDesc := []mkvcore.TrackDescription{
		{
			TrackNumber: 1,
			TrackEntry: webm.TrackEntry{
				Name:        trackNameAudio,
				TrackNumber: 1,
				TrackUID:    1,
				CodecID:     codecIDOPUS,
				TrackType:   trackTypeAudio,
				Audio: &webm.Audio{
					SamplingFrequency: w.audioSampleRate,
					Channels:          w.audioChannels,
				},
			},
		},
		{
			TrackNumber: webmVideoTrackNumber,
			TrackEntry: webm.TrackEntry{
				TrackNumber:     webmVideoTrackNumber,
				TrackUID:        webmVideoTrackNumber,
				TrackType:       trackTypeVideo,
				Name:            trackNameVideo,
				CodecID:         codecIDH264,
				DefaultDuration: 0,
			},
		},
	}

	var err error
	w.tempFile, err = ioutil.TempFile("/tmp", "ebmlmuxer-*.mkv")
	if err != nil {
		return nil, err
	}

	writers, err := mkvcore.NewSimpleBlockWriter(w.tempFile, mkvTrackDesc,
		mkvcore.WithSeekHead(true),
		mkvcore.WithEBMLHeader(mkv.DefaultEBMLHeader),
		mkvcore.WithSegmentInfo(&webm.Info{
			TimecodeScale: defaultTimecode,
			MuxingApp:     "your_app_name",
			WritingApp:    "your_app_name",
			Duration:      defaultDuration,
		}),
		mkvcore.WithMarshalOptions(ebml.WithElementWriteHooks(func(e *ebml.Element) {
			if e.Name == "Duration" {
				w.durationPos = int64(e.Position + 4)
			}
		})),
	)
	if err != nil {
		return nil, err
	}
	return writers, nil
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

func (w *EBMLMuxer) Finalize(ctx context.Context, output io.Writer) error {
	defer w.cleanup()

	if err := w.overwritePTS(); err != nil {
		return fmt.Errorf("overwrite PTS error: %w", err)
	}

	// Copy the data from the temporary file to the output writer
	if _, err := w.tempFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek error: %w", err)
	}
	if _, err := io.Copy(output, w.tempFile); err != nil {
		return fmt.Errorf("copy error: %w", err)
	}

	for _, writer := range w.writers {
		if err := writer.Close(); err != nil {
			return fmt.Errorf("writer close error: %w", err)
		}
	}
	return nil
}

func (w *EBMLMuxer) overwritePTS() error {
	ptsBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(ptsBytes, math.Float64bits(float64(w.duration)))
	if _, err := w.tempFile.Seek(w.durationPos, io.SeekStart); err != nil {
		return err
	}
	if _, err := w.tempFile.Write(ptsBytes); err != nil {
		return err
	}
	return nil
}

func (w *EBMLMuxer) cleanup() {
	if w.tempFile != nil {
		w.tempFile.Close()
		//os.Remove(w.tempFile.Name())
		w.tempFile = nil
	}
	w.writers = nil
	w.duration = 0
	w.durationPos = 0
}

func (w *EBMLMuxer) ContainerName() string {
	return string(w.container)
}
