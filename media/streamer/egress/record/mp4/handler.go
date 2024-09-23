package mp4

import "C"
import (
	"context"
	"errors"
	"fmt"
	"liveflow/media/streamer/egress/record"
	"liveflow/media/streamer/processes"
	"os"
	"time"

	astiav "github.com/asticode/go-astiav"
	"github.com/deepch/vdk/codec/aacparser"
	"github.com/sirupsen/logrus"
	gomp4 "github.com/yapingcat/gomedia/go-mp4"

	"liveflow/log"
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

type MP4 struct {
	hub                   *hub.Hub
	muxer                 *gomp4.Movmuxer
	tempFile              *os.File
	hasVideo              bool
	videoIndex            uint32
	hasAudio              bool
	audioIndex            uint32
	mpeg4AudioConfigBytes []byte
	mpeg4AudioConfig      *aacparser.MPEG4AudioConfig
	streamID              string

	// New fields for splitting
	splitIntervalMS int64
	lastSplitTime   int64
	fileIndex       int
	splitPending    bool // Indicates if a split is pending
}

type MP4Args struct {
	Hub             *hub.Hub
	SplitIntervalMS int64
}

func NewMP4(args MP4Args) *MP4 {
	return &MP4{
		hub:             args.Hub,
		splitIntervalMS: args.SplitIntervalMS,
	}
}

func (m *MP4) Start(ctx context.Context, source hub.Source) error {
	if !hub.HasCodecType(source.MediaSpecs(), hub.CodecTypeAAC) && !hub.HasCodecType(source.MediaSpecs(), hub.CodecTypeOpus) {
		return ErrUnsupportedCodec
	}
	if !hub.HasCodecType(source.MediaSpecs(), hub.CodecTypeH264) {
		return ErrUnsupportedCodec
	}
	m.streamID = source.StreamID()
	ctx = log.WithFields(ctx, logrus.Fields{
		fields.StreamID:   source.StreamID(),
		fields.SourceName: source.Name(),
	})
	log.Info(ctx, "start mp4")
	sub := m.hub.Subscribe(source.StreamID())
	go func() {
		var err error

		// Initialize the splitting logic
		m.fileIndex = 0
		err = m.createNewFile(ctx)
		if err != nil {
			log.Error(ctx, err, "failed to create mp4 file")
			return
		}
		defer m.closeFile(ctx)

		var audioTranscodingProcess *processes.AudioTranscodingProcess
		for data := range sub {
			// Check if we need to initiate a split
			if data.H264Video != nil {
				if !m.splitPending && data.H264Video.RawDTS()-m.lastSplitTime >= m.splitIntervalMS {
					m.splitPending = true
				}

			}

			if data.H264Video != nil {
				m.onVideo(ctx, data.H264Video)
			}
			if data.OPUSAudio != nil {
				if audioTranscodingProcess == nil {
					audioTranscodingProcess = processes.NewTranscodingProcess(astiav.CodecIDOpus, astiav.CodecIDAac, audioSampleRate)
					audioTranscodingProcess.Init()
					defer audioTranscodingProcess.Close()
					m.mpeg4AudioConfigBytes = audioTranscodingProcess.ExtraData()
					tmpAudioCodec, err := aacparser.NewCodecDataFromMPEG4AudioConfigBytes(m.mpeg4AudioConfigBytes)
					if err != nil {
						log.Error(ctx, err)
					}
					m.mpeg4AudioConfig = &tmpAudioCodec.Config
				}
				m.onOPUSAudio(ctx, audioTranscodingProcess, data.OPUSAudio)
			} else {
				if data.AACAudio != nil {
					m.onAudio(ctx, data.AACAudio)
				}
			}
		}
		err = m.muxer.WriteTrailer()
		if err != nil {
			log.Error(ctx, err, "failed to write trailer")
		}
	}()
	return nil
}

// createNewFile creates a new MP4 file and initializes the muxer
func (m *MP4) createNewFile(ctx context.Context) error {
	var err error
	m.closeFile(ctx) // Close previous file if any
	timestamp := time.Now().Format("2006-01-02-15-04-05")
	fileName := fmt.Sprintf("videos/%s_%s.mp4", m.streamID, timestamp)
	m.tempFile, err = record.CreateFileInDir(fileName)
	if err != nil {
		return err
	}
	m.muxer, err = gomp4.CreateMp4Muxer(m.tempFile)
	if err != nil {
		return err
	}
	m.hasVideo = false
	m.hasAudio = false
	m.videoIndex = 0
	m.audioIndex = 0
	m.lastSplitTime = 0
	m.fileIndex++
	return nil
}

// closeFile closes the current MP4 file and muxer
func (m *MP4) closeFile(ctx context.Context) {
	if m.muxer != nil {
		err := m.muxer.WriteTrailer()
		if err != nil {
			log.Error(ctx, err, "failed to write trailer")
		}
		m.muxer = nil
	}
	if m.tempFile != nil {
		err := m.tempFile.Close()
		if err != nil {
			log.Error(ctx, err, "failed to close mp4 file")
		}
		m.tempFile = nil
	}
}

// splitFile handles the logic to split the MP4 file
func (m *MP4) splitFile(ctx context.Context) error {
	// Close current file
	m.closeFile(ctx)
	// Create a new file
	return m.createNewFile(ctx)
}

func (m *MP4) onVideo(ctx context.Context, h264Video *hub.H264Video) {
	// Check if this is a keyframe
	isKeyFrame := false
	for _, sliceType := range h264Video.SliceTypes {
		if sliceType == hub.SliceI {
			isKeyFrame = true
			break
		}
	}

	// If a split is pending and we have a keyframe, perform the split
	if m.splitPending && isKeyFrame {
		err := m.splitFile(ctx)
		if err != nil {
			log.Error(ctx, err, "failed to split mp4 file")
			return
		}
		m.lastSplitTime = h264Video.RawDTS()
		m.splitPending = false // Reset the split pending flag
	}

	if !m.hasVideo {
		m.hasVideo = true
		m.videoIndex = m.muxer.AddVideoTrack(gomp4.MP4_CODEC_H264)
	}

	videoData := make([]byte, len(h264Video.Data))
	copy(videoData, h264Video.Data)
	err := m.muxer.Write(m.videoIndex, videoData, uint64(h264Video.RawPTS()-m.lastSplitTime), uint64(h264Video.RawDTS()-m.lastSplitTime))
	if err != nil {
		log.Error(ctx, err, "failed to write video")
	}
}

func (m *MP4) onAudio(ctx context.Context, aacAudio *hub.AACAudio) {
	if !m.hasAudio {
		m.hasAudio = true
		m.audioIndex = m.muxer.AddAudioTrack(gomp4.MP4_CODEC_AAC)
	}
	if len(aacAudio.MPEG4AudioConfigBytes) > 0 {
		m.mpeg4AudioConfigBytes = aacAudio.MPEG4AudioConfigBytes
	}
	if aacAudio.MPEG4AudioConfig != nil {
		m.mpeg4AudioConfig = aacAudio.MPEG4AudioConfig
	}
	if len(aacAudio.Data) > 0 && m.mpeg4AudioConfig != nil {
		var audioData []byte
		const (
			aacSamples     = 1024
			adtsHeaderSize = 7
		)
		adtsHeader := make([]byte, adtsHeaderSize)
		aacparser.FillADTSHeader(adtsHeader, *m.mpeg4AudioConfig, aacSamples, len(aacAudio.Data))
		audioData = append(adtsHeader, aacAudio.Data...)
		err := m.muxer.Write(m.audioIndex, audioData, uint64(aacAudio.RawPTS()-m.lastSplitTime), uint64(aacAudio.RawDTS()-m.lastSplitTime))
		if err != nil {
			log.Error(ctx, err, "failed to write audio")
		}
	}
}

func (m *MP4) onOPUSAudio(ctx context.Context, audioTranscodingProcess *processes.AudioTranscodingProcess, opusAudio *hub.OPUSAudio) {
	packets, err := audioTranscodingProcess.Process(&processes.MediaPacket{
		Data: opusAudio.Data,
		PTS:  opusAudio.PTS,
		DTS:  opusAudio.DTS,
	})
	if err != nil {
		fmt.Println(err)
	}
	for _, packet := range packets {
		m.onAudio(ctx, &hub.AACAudio{
			Data:                  packet.Data,
			SequenceHeader:        false,
			MPEG4AudioConfigBytes: m.mpeg4AudioConfigBytes,
			MPEG4AudioConfig:      m.mpeg4AudioConfig,
			PTS:                   packet.PTS,
			DTS:                   packet.DTS,
			AudioClockRate:        uint32(packet.SampleRate),
		})
	}
}
