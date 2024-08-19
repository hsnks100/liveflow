package mp4

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/deepch/vdk/codec/h264parser"
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
	hub        *hub.Hub
	muxer      *gomp4.Movmuxer
	tempFile   *os.File
	videoIndex uint32
	audioIndex uint32
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
	//h.audioIndex = mp4Muxer.AddAudioTrack(gomp4.MP4_CODEC_AAC)

	go func() {
		var err error
		mp4FileName := fmt.Sprintf("%s_%s.mp4", streamID, time.Now().Format("20060102150405"))
		mp4File, err := os.OpenFile(mp4FileName, os.O_CREATE|os.O_RDWR, 0666)
		if err != nil {
			fmt.Println(err)
			return
		}
		defer mp4File.Close()
		fmt.Println(mp4File.Seek(0, io.SeekCurrent))
		cws := newCacheWriterSeeker(4096)
		muxer, err := gomp4.CreateMp4Muxer(cws)
		if err != nil {
			fmt.Println(err)
			return
		}
		vtid := muxer.AddVideoTrack(gomp4.MP4_CODEC_H264)

		h.muxer = muxer
		h.videoIndex = vtid
		for data := range sub {
			//fmt.Println("@@@ MP4")
			if data.H264Video != nil {
				//fmt.Printf("MP4: %d, size: %d\n", data.H264Video.Timestamp, len(data.H264Video.Data))
				if data.H264Video.SliceType == h264parser.SLICE_I {
					err := h.muxer.Write(h.videoIndex, data.H264Video.SPS, uint64(data.H264Video.PTS), uint64(data.H264Video.DTS))
					if err != nil {
						log.Error(ctx, err, "failed to write video")
					}
					err = h.muxer.Write(h.videoIndex, data.H264Video.PPS, uint64(data.H264Video.PTS), uint64(data.H264Video.DTS))
					if err != nil {
						log.Error(ctx, err, "failed to write video")
					}
					err = h.muxer.Write(h.videoIndex, data.H264Video.Data, uint64(data.H264Video.PTS), uint64(data.H264Video.DTS))
					if err != nil {
						log.Error(ctx, err, "failed to write video")
					}
				} else {
					err := h.muxer.Write(h.videoIndex, data.H264Video.Data, uint64(data.H264Video.PTS), uint64(data.H264Video.DTS))
					if err != nil {
						log.Error(ctx, err, "failed to write video")
					}
				}
			}
			if data.AACAudio != nil {
				//fmt.Printf("MP4: %d\n", data.AACAudio.Timestamp)
			}
		}
		err = muxer.WriteTrailer()
		if err != nil {
			panic(err)
		}
		fmt.Println("video len: ", len(cws.buf))
		_, err = mp4File.Write(cws.buf)
		if err != nil {
			panic(err)
		}
	}()
	return nil
}
