package processes

import (
	"context"
	"errors"
	"fmt"
	astiav "liveflow/goastiav"
	"liveflow/log"
	"liveflow/media/hub"
	"liveflow/media/streamer/pipe"
)

type VideoDecodingProcess struct {
	pipe.BaseProcess[hub.H264Video, []*astiav.Frame]

	codecID         astiav.CodecID
	decCodec        *astiav.Codec
	decCodecContext *astiav.CodecContext
}

func NewVideoDecodingProcess(codecID astiav.CodecID) *VideoDecodingProcess {
	return &VideoDecodingProcess{
		codecID: codecID,
	}
}

func (v *VideoDecodingProcess) Init() error {
	fmt.Println("@@@ VideoDecodingProcess Init")
	// Create a new codec
	v.decCodec = astiav.FindDecoder(v.codecID)

	// Create a new codec context
	v.decCodecContext = astiav.AllocCodecContext(v.decCodec)

	// Open codec context
	if err := v.decCodecContext.Open(v.decCodec, nil); err != nil {
		return err
	}

	return nil
}
func (v *VideoDecodingProcess) Process(data hub.H264Video) ([]*astiav.Frame, error) {
	// Decode data
	ctx := context.Background()
	packet := astiav.AllocPacket()
	//defer packet.Free()
	err := packet.FromData(data.Data)
	if err != nil {
		log.Error(ctx, err, "failed to create packet")
	}
	err = v.decCodecContext.SendPacket(packet)
	if err != nil {
		log.Error(ctx, err, "failed to send packet")
	}
	var frames []*astiav.Frame
	for {
		frame := astiav.AllocFrame()
		err := v.decCodecContext.ReceiveFrame(frame)
		if errors.Is(err, astiav.ErrEof) {
			fmt.Println("EOF: ", err.Error())
			break
		} else if errors.Is(err, astiav.ErrEagain) {
			break
		}
		frames = append(frames, frame)
	}

	return frames, nil
}
