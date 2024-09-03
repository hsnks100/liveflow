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

type TranscodingProcess struct {
	pipe.BaseProcess[hub.AACAudio, []*hub.OPUSAudio]
	codecID         astiav.CodecID
	decCodec        *astiav.Codec
	decCodecContext *astiav.CodecContext
	encCodec        *astiav.Codec
	encCodecContext *astiav.CodecContext
}

func (t *TranscodingProcess) Init() error {
	t.decCodec = astiav.FindDecoder(t.codecID)
	if t.decCodec == nil {
		return errors.New("codec is nil")
	}
	t.decCodecContext = astiav.AllocCodecContext(t.decCodec)
	if t.decCodecContext == nil {
		return errors.New("codec context is nil")
	}
	if err := t.decCodecContext.Open(t.decCodec, nil); err != nil {
		return err
	}

	t.encCodec = astiav.FindEncoder(astiav.CodecIDOpus)
	if t.encCodec == nil {
		return errors.New("codec is nil")
	}
	t.encCodecContext = astiav.AllocCodecContext(t.encCodec)
	if t.encCodecContext == nil {
		return errors.New("codec context is nil")
	}
	if t.decCodecContext.MediaType() == astiav.MediaTypeAudio {
		if v := t.encCodec.ChannelLayouts(); len(v) > 0 {
			t.encCodecContext.SetChannelLayout(v[0])
		} else {
			t.encCodecContext.SetChannelLayout(t.decCodecContext.ChannelLayout())
		}
		t.encCodecContext.SetSampleRate(t.decCodecContext.SampleRate())
		if v := t.encCodec.SampleFormats(); len(v) > 0 {
			t.encCodecContext.SetSampleFormat(v[0])
		} else {
			t.encCodecContext.SetSampleFormat(t.decCodecContext.SampleFormat())
		}
		t.encCodecContext.SetTimeBase(astiav.NewRational(1, t.encCodecContext.SampleRate()))
	} else {
		t.encCodecContext.SetHeight(t.decCodecContext.Height())
		if v := t.encCodec.PixelFormats(); len(v) > 0 {
			t.encCodecContext.SetPixelFormat(v[0])
		} else {
			t.encCodecContext.SetPixelFormat(t.decCodecContext.PixelFormat())
		}
		t.encCodecContext.SetSampleAspectRatio(t.decCodecContext.SampleAspectRatio())
		frameRate := t.decCodecContext.Framerate()
		t.encCodecContext.SetTimeBase(astiav.NewRational(frameRate.Den(), frameRate.Num()))
		t.encCodecContext.SetWidth(t.decCodecContext.Width())
	}
	return nil
}

func (t *TranscodingProcess) Process(data hub.AACAudio) ([]*hub.OPUSAudio, error) {
	ctx := context.Background()
	packet := astiav.AllocPacket()
	//defer packet.Free()
	err := packet.FromData(data.Data)
	if err != nil {
		log.Error(ctx, err, "failed to create packet")
	}
	err = t.decCodecContext.SendPacket(packet)
	if err != nil {
		log.Error(ctx, err, "failed to send packet")
	}
	var opusAudio []*hub.OPUSAudio
	for {
		frame := astiav.AllocFrame()
		err := t.decCodecContext.ReceiveFrame(frame)
		if errors.Is(err, astiav.ErrEof) {
			fmt.Println("EOF: ", err.Error())
			break
		} else if errors.Is(err, astiav.ErrEagain) {
			break
		}

		// Encode data
		err = t.encCodecContext.SendFrame(frame)
		if err != nil {
			log.Error(ctx, err, "failed to send frame")
		}
		for {
			pkt := astiav.AllocPacket()
			err := t.encCodecContext.ReceivePacket(pkt)
			if errors.Is(err, astiav.ErrEof) {
				fmt.Println("EOF: ", err.Error())
				break
			} else if errors.Is(err, astiav.ErrEagain) {
				break
			}
			opusAudio = append(opusAudio, &hub.OPUSAudio{
				PTS:            pkt.Pts(),
				DTS:            pkt.Dts(),
				Data:           pkt.Data(),
				AudioClockRate: uint32(t.encCodecContext.SampleRate()),
			})
		}
	}
	return opusAudio, nil
}
