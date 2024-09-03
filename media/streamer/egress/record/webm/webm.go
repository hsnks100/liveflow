package webm

import (
	"context"
	"encoding/binary"
	"errors"
	"liveflow/log"
	"math"
	"os"

	"github.com/at-wat/ebml-go"
	"github.com/at-wat/ebml-go/mkv"
	"github.com/at-wat/ebml-go/mkvcore"
	"github.com/at-wat/ebml-go/webm"
)

var errInitFailed = errors.New("init failed")

const (
	webmAudioStream      = 0
	webmAudioTrackNumber = 1
	webmVideoStream      = 1
	webmVideoTrackNumber = 2

	trackTypeVideo = 1
	trackTypeAudio = 2

	trackNameVideo  = "Video"
	trackNameAudio  = "Audio"
	audioSampleRate = 48000
	defaultDuration = 600_000
	defaultTimecode = 1_000_000
)

const (
	codecIDVP8  = "V_VP8"
	codecIDH264 = "V_MPEG4/ISO/AVC"
	codecIDOPUS = "A_OPUS"
	codecIDAAC  = "A_AAC"
)

type Name string

const (
	WebM Name = "webm"
	MP4  Name = "mp4"
	MKV  Name = "mkv"

	Default Name = "default" // Record 에서 특별하게 사용
)
const durationElement = "Duration"

type webmMuxer struct {
	writers         []mkvcore.BlockWriteCloser
	container       Name
	tempFileName    string
	audioSampleRate float64
	audioChannels   uint64
	durationPos     uint64
	duration        int64
}

func NewWebmMuxer(sampleRate int, channels int, container Name) *webmMuxer {
	return &webmMuxer{
		writers:         nil,
		tempFileName:    "",
		audioSampleRate: float64(sampleRate),
		audioChannels:   uint64(channels),
		durationPos:     0,
		duration:        0,
		container:       container,
	}
}

func (w *webmMuxer) makeWebmWriters() ([]mkvcore.BlockWriteCloser, error) {
	tempFile, err := os.CreateTemp("", "*.webm")
	if err != nil {
		return nil, err
	}
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
			Duration:      defaultDuration, // 임의로 default videoSplitIntervalMs 로 설정. writeTrailer에서 최종 값으로 수정됨.
		}),
		mkvcore.WithMarshalOptions(ebml.WithElementWriteHooks(func(e *ebml.Element) {
			switch e.Name {
			case durationElement:
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

func (w *webmMuxer) makeMKVWriters() ([]mkvcore.BlockWriteCloser, error) {
	tempFile, err := os.CreateTemp("./", "*.mkv")
	if err != nil {
		return nil, err
	}
	var mkvTrackDesc []mkvcore.TrackDescription

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
			// TODO: 해상도를 나중에 write 하는 방식으로 바꿔야 할 것 같음, 일단 없어도 잘 돔.
			//Video: &webm.Video{
			//	PixelWidth:  1280,
			//	PixelHeight: 720,
			//},
		},
	})
	var mkvWriters []mkvcore.BlockWriteCloser
	mkvWriters, err = mkvcore.NewSimpleBlockWriter(tempFile, mkvTrackDesc,
		mkvcore.WithEBMLHeader(mkv.DefaultEBMLHeader),
		mkvcore.WithSeekHead(true),
		mkvcore.WithSegmentInfo(&webm.Info{
			TimecodeScale: defaultTimecode, // 1ms
			MuxingApp:     "mrw-v4.ebml-go.mkv",
			WritingApp:    "mrw-v4.ebml-go.mkv",
			Duration:      defaultDuration, // 임의로 default videoSplitIntervalMs 로 설정. writeTrailer에서 최종 값으로 수정됨.
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

func (w *webmMuxer) Init(_ context.Context) error {
	var err error
	switch w.container {
	case WebM:
		w.writers, err = w.makeWebmWriters()
	case MKV:
		w.writers, err = w.makeMKVWriters()
	default:
		return errInitFailed
	}
	return err
}

func (w *webmMuxer) Initialized() bool {
	return len(w.writers) > 0
}

func (w *webmMuxer) WriteVideo(data []byte, keyframe bool, pts uint64, _ uint64) error {
	if _, err := w.writers[webmVideoStream].Write(keyframe, int64(pts), data); err != nil {
		return err
	}
	if w.duration < int64(pts) {
		w.duration = int64(pts)
	}
	return nil
}

func (w *webmMuxer) WriteAudio(data []byte, keyframe bool, pts uint64, _ uint64) error {
	if _, err := w.writers[webmAudioStream].Write(keyframe, int64(pts), data); err != nil {
		return err
	}
	if w.duration < int64(pts) {
		w.duration = int64(pts)
	}
	return nil
}

func (w *webmMuxer) Finalize(ctx context.Context) error {
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

func (w *webmMuxer) ContainerName() string {
	return string(w.container)
}

func (w *webmMuxer) overwritePTS(ctx context.Context, fileName string) error {
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

func (w *webmMuxer) cleanup() {
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
