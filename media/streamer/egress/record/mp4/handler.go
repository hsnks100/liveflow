package mp4

import "C"
import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"

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
	fmt.Println("@@@ Write: ", len(p))
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
	//
}

type MP4Args struct {
	Hub *hub.Hub
}

func NewMP4(args MP4Args) *MP4 {
	return &MP4{
		hub: args.Hub,
	}
}

func (h *MP4) Start(ctx context.Context, source hub.Source) error {
	containsAudio := false
	containsVideo := false
	audioCodec := ""
	videoCodec := ""
	for _, spec := range source.MediaSpecs() {
		if spec.MediaType == hub.Audio {
			containsAudio = true
			audioCodec = spec.CodecType
		}
		if spec.MediaType == hub.Video {
			containsVideo = true
			videoCodec = spec.CodecType
		}
	}
	if !containsVideo || !containsAudio {
		return ErrNotContainAudioOrVideo
	}
	if audioCodec != "aac" {
		return ErrUnsupportedCodec
	}
	if videoCodec != "h264" {
		return ErrUnsupportedCodec
	}

	ctx = log.WithFields(ctx, logrus.Fields{
		fields.StreamID:   source.StreamID(),
		fields.SourceName: source.Name(),
	})
	log.Info(ctx, "start mp4")
	sub := h.hub.Subscribe(source.StreamID())

	go func() {
		var err error
		mp4File, err := os.CreateTemp("./", "*.mp4")
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
		h.muxer = muxer

		for data := range sub {
			if data.AACAudio != nil {
				log.Debug(ctx, "AACAudio: ", data.AACAudio.RawPTS())
			}
			if data.H264Video != nil {
				log.Debug(ctx, "H264Video: ", data.H264Video.RawPTS())
			}
			if data.H264Video != nil {
				if !h.hasVideo {
					h.hasVideo = true
					h.videoIndex = muxer.AddVideoTrack(gomp4.MP4_CODEC_H264)
				}
				//fmt.Println(hex.Dump(data.H264Video.Data))
				videoData := make([]byte, len(data.H264Video.Data))
				copy(videoData, data.H264Video.Data)
				err = h.muxer.Write(h.videoIndex, videoData, uint64(data.H264Video.RawPTS()), uint64(data.H264Video.RawDTS()))
				if err != nil {
					log.Error(ctx, err, "failed to write video")
				}
			}
			if data.AACAudio != nil {
				if !h.hasAudio {
					h.hasAudio = true
					h.audioIndex = muxer.AddAudioTrack(gomp4.MP4_CODEC_AAC)
				}
				if len(data.AACAudio.MPEG4AudioConfigBytes) > 0 {
					fmt.Println("@@@ set mpeg4AudioConfigBytes")
					h.mpeg4AudioConfigBytes = data.AACAudio.MPEG4AudioConfigBytes
				}
				if data.AACAudio.MPEG4AudioConfig != nil {
					fmt.Println("@@@ set mpeg4AudioConfig")
					h.mpeg4AudioConfig = data.AACAudio.MPEG4AudioConfig
				}
				if len(data.AACAudio.Data) > 0 && h.mpeg4AudioConfig != nil {
					var audioData []byte
					const (
						aacSamples     = 1024
						adtsHeaderSize = 7
					)
					adtsHeader := make([]byte, adtsHeaderSize)
					aacparser.FillADTSHeader(adtsHeader, *h.mpeg4AudioConfig, aacSamples, len(data.AACAudio.Data))
					audioData = append(adtsHeader, data.AACAudio.Data...)
					err := h.muxer.Write(h.audioIndex, audioData, uint64(data.AACAudio.RawPTS()), uint64(data.AACAudio.RawDTS()))
					if err != nil {
						log.Error(ctx, err, "failed to write audio")
					}
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

func saveAsJPEG(img image.Image, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	options := &jpeg.Options{
		Quality: 90, // JPEG 품질 (1~100)
	}
	return jpeg.Encode(file, img, options)
}
