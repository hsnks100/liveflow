package mp4

import "C"
import (
	"context"
	"errors"
	"fmt"
	"io"
	"liveflow/media/streamer/processes"
	"os"

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

type cacheWriterSeeker struct {
	buf    []byte
	offset int
}

func newCacheWriterSeeker(capacity int) *cacheWriterSeeker {
	return &cacheWriterSeeker{
		buf:    make([]byte, 0, capacity),
		offset: 0,
	}
}

func (ws *cacheWriterSeeker) Write(p []byte) (n int, err error) {
	if cap(ws.buf)-ws.offset >= len(p) {
		if len(ws.buf) < ws.offset+len(p) {
			ws.buf = ws.buf[:ws.offset+len(p)]
		}
		copy(ws.buf[ws.offset:], p)
		ws.offset += len(p)
		return len(p), nil
	}
	tmp := make([]byte, len(ws.buf), cap(ws.buf)+len(p)*2)
	copy(tmp, ws.buf)
	if len(ws.buf) < ws.offset+len(p) {
		tmp = tmp[:ws.offset+len(p)]
	}
	copy(tmp[ws.offset:], p)
	ws.buf = tmp
	ws.offset += len(p)
	return len(p), nil
}

func (ws *cacheWriterSeeker) Seek(offset int64, whence int) (int64, error) {
	if whence == io.SeekCurrent {
		if ws.offset+int(offset) > len(ws.buf) {
			return -1, errors.New(fmt.Sprint("SeekCurrent out of range", len(ws.buf), offset, ws.offset))
		}
		ws.offset += int(offset)
		return int64(ws.offset), nil
	} else if whence == io.SeekStart {
		if offset > int64(len(ws.buf)) {
			return -1, errors.New(fmt.Sprint("SeekStart out of range", len(ws.buf), offset, ws.offset))
		}
		ws.offset = int(offset)
		return offset, nil
	} else {
		return 0, errors.New("unsupport SeekEnd")
	}
}

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
}

type MP4Args struct {
	Hub *hub.Hub
}

func NewMP4(args MP4Args) *MP4 {
	return &MP4{
		hub: args.Hub,
	}
}

func (m *MP4) Start(ctx context.Context, source hub.Source) error {
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
	var audioTranscodingProcess *processes.AudioTranscodingProcess
	if hub.HasCodecType(source.MediaSpecs(), hub.CodecTypeOpus) {
		audioTranscodingProcess = processes.NewTranscodingProcess(astiav.CodecIDOpus, astiav.CodecIDAac, audioSampleRate)
		audioTranscodingProcess.Init()
		m.mpeg4AudioConfigBytes = audioTranscodingProcess.ExtraData()
		tmpAudioCodec, err := aacparser.NewCodecDataFromMPEG4AudioConfigBytes(m.mpeg4AudioConfigBytes)
		if err != nil {
			return err
		}
		m.mpeg4AudioConfig = &tmpAudioCodec.Config
	}
	log.Info(ctx, "start mp4")
	sub := m.hub.Subscribe(source.StreamID())
	go func() {
		var err error
		mp4File, err := os.CreateTemp("./", fmt.Sprintf("%s-*.mp4", source.StreamID()))
		if err != nil {
			fmt.Println(err)
			return
		}
		defer func() {
			err := mp4File.Close()
			if err != nil {
				log.Error(ctx, err, "failed to close mp4 file")
			}
		}()
		muxer, err := gomp4.CreateMp4Muxer(mp4File)
		if err != nil {
			fmt.Println(err)
			return
		}
		m.muxer = muxer

		for data := range sub {
			if data.H264Video != nil {
				m.onVideo(ctx, data.H264Video)
			}
			if data.OPUSAudio != nil {
				m.onOPUSAudio(ctx, audioTranscodingProcess, data.OPUSAudio)
			} else {
				if data.AACAudio != nil {
					m.onAudio(ctx, data.AACAudio)
				}
			}
		}
		err = muxer.WriteTrailer()
		if err != nil {
			panic(err)
		}
	}()
	return nil
}

func (m *MP4) onVideo(ctx context.Context, h264Video *hub.H264Video) {
	if !m.hasVideo {
		m.hasVideo = true
		m.videoIndex = m.muxer.AddVideoTrack(gomp4.MP4_CODEC_H264)
	}
	videoData := make([]byte, len(h264Video.Data))
	copy(videoData, h264Video.Data)
	err := m.muxer.Write(m.videoIndex, videoData, uint64(h264Video.RawPTS()), uint64(h264Video.RawDTS()))
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
		err := m.muxer.Write(m.audioIndex, audioData, uint64(aacAudio.RawPTS()), uint64(aacAudio.RawDTS()))
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
