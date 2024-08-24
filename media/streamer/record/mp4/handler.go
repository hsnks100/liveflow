package mp4

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/deepch/vdk/codec/aacparser"
	gomp4 "github.com/yapingcat/gomedia/go-mp4"

	"mrw-clone/log"
	"mrw-clone/media/hub"
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
	videoIndex            uint32
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

func (h *MP4) Start(ctx context.Context, streamID string) error {
	sub := h.hub.Subscribe(streamID)

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
		h.videoIndex = muxer.AddVideoTrack(gomp4.MP4_CODEC_H264)
		h.audioIndex = muxer.AddAudioTrack(gomp4.MP4_CODEC_AAC)
		h.muxer = muxer
		for data := range sub {
			fmt.Println("@@@ MP4")
			if data.AACAudio != nil {
				fmt.Println("[MP4] AACAudio: ", data.AACAudio.RawDTS())
			}
			if data.H264Video != nil {
				fmt.Println("[MP4] H264Video: ", data.H264Video.RawDTS())
			}
			if data.H264Video != nil {
				//fmt.Println(hex.Dump(data.H264Video.Data))
				videoData := make([]byte, len(data.H264Video.Data))
				copy(videoData, data.H264Video.Data)
				err := h.muxer.Write(h.videoIndex, videoData, uint64(data.H264Video.RawPTS()), uint64(data.H264Video.RawDTS()))
				if err != nil {
					log.Error(ctx, err, "failed to write video")
				}
			}
			if data.AACAudio != nil {
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
