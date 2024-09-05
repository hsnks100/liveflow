package processes

import (
	"context"
	"errors"
	"fmt"
	"liveflow/log"
	"liveflow/media/streamer/pipe"

	astiav "github.com/asticode/go-astiav"
)

type MediaPacket struct {
	Data       []byte
	PTS        int64
	DTS        int64
	SampleRate int
}
type AudioTranscodingProcess struct {
	pipe.BaseProcess[*MediaPacket, []*MediaPacket]
	decCodecID      astiav.CodecID
	encCodecID      astiav.CodecID
	decCodec        *astiav.Codec
	decCodecContext *astiav.CodecContext
	encCodec        *astiav.Codec
	encCodecContext *astiav.CodecContext
	encSampleRate   int

	audioFifo *astiav.AudioFifo
	lastPts   int64
	//nbSamples int
}

func NewTranscodingProcess(decCodecID astiav.CodecID, encCodecID astiav.CodecID, encSampleRate int) *AudioTranscodingProcess {
	return &AudioTranscodingProcess{
		decCodecID:    decCodecID,
		encCodecID:    encCodecID,
		encSampleRate: encSampleRate,
	}
}

func (t *AudioTranscodingProcess) ExtraData() []byte {
	return t.encCodecContext.ExtraData()
}

func (t *AudioTranscodingProcess) Init() error {
	t.decCodec = astiav.FindDecoder(t.decCodecID)
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

	if t.encCodecID == astiav.CodecIDOpus {
		t.encCodec = astiav.FindEncoderByName("opus")
	} else {
		t.encCodec = astiav.FindEncoder(t.encCodecID)
	}
	if t.encCodec == nil {
		return errors.New("codec is nil")
	}
	t.encCodecContext = astiav.AllocCodecContext(t.encCodec)
	if t.encCodecContext == nil {
		return errors.New("codec context is nil")
	}
	if t.decCodecContext.MediaType() == astiav.MediaTypeAudio {
		t.encCodecContext.SetChannelLayout(astiav.ChannelLayoutStereo)
		t.encCodecContext.SetSampleRate(t.encSampleRate)
		t.encCodecContext.SetSampleFormat(astiav.SampleFormatFltp) // t.encCodec.SampleFormats()[0])
		t.encCodecContext.SetBitRate(64000)
		//t.encCodecContext.SetTimeBase(astiav.NewRational(1, t.encCodecContext.SampleRate()))
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
	dict := astiav.NewDictionary()
	dict.Set("strict", "-2", 0)
	if err := t.encCodecContext.Open(t.encCodec, dict); err != nil {
		return err
	}
	fmt.Println("framesize2 : ", t.encCodecContext.FrameSize())
	return nil
}

func (t *AudioTranscodingProcess) Process(data *MediaPacket) ([]*MediaPacket, error) {
	ctx := context.Background()
	packet := astiav.AllocPacket()
	//defer packet.Free()
	err := packet.FromData(data.Data)
	if err != nil {
		log.Error(ctx, err, "failed to create packet")
	}
	packet.SetPts(data.PTS)
	packet.SetDts(data.DTS)
	err = t.decCodecContext.SendPacket(packet)
	if err != nil {
		log.Error(ctx, err, "failed to send packet")
	}
	if t.audioFifo == nil {
		t.audioFifo = astiav.AllocAudioFifo(
			t.encCodecContext.SampleFormat(),
			t.encCodecContext.ChannelLayout().Channels(),
			t.encCodecContext.SampleRate())
	}
	var opusAudio []*MediaPacket
	for {
		frame := astiav.AllocFrame()
		defer frame.Free()
		err := t.decCodecContext.ReceiveFrame(frame)
		if errors.Is(err, astiav.ErrEof) {
			fmt.Println("EOF: ", err.Error())
			break
		} else if errors.Is(err, astiav.ErrEagain) {
			break
		}
		t.audioFifo.Write(frame)
		nbSamples := 0
		for t.audioFifo.Size() >= t.encCodecContext.FrameSize() {
			frameToSend := astiav.AllocFrame()
			frameToSend.SetNbSamples(t.encCodecContext.FrameSize())
			frameToSend.SetChannelLayout(t.encCodecContext.ChannelLayout()) // t.encCodecContext.ChannelLayout())
			frameToSend.SetSampleFormat(t.encCodecContext.SampleFormat())
			frameToSend.SetSampleRate(t.encCodecContext.SampleRate())
			frameToSend.SetPts(t.lastPts + int64(t.encCodecContext.FrameSize()))
			t.lastPts = frameToSend.Pts()
			nbSamples += frame.NbSamples()
			err := frameToSend.AllocBuffer(0)
			if err != nil {
				log.Error(ctx, err, "failed to alloc buffer")
			}
			read, err := t.audioFifo.Read(frameToSend)
			if err != nil {
				log.Error(ctx, err, "failed to read fifo")
			}
			if read < frameToSend.NbSamples() {
				log.Error(ctx, err, "failed to read fifo")
			}
			// Encode the frame
			err = t.encCodecContext.SendFrame(frameToSend)
			if err != nil {
				log.Error(ctx, err, "failed to send frame")
			}
			for {
				pkt := astiav.AllocPacket()
				defer pkt.Free()
				err := t.encCodecContext.ReceivePacket(pkt)
				if errors.Is(err, astiav.ErrEof) {
					fmt.Println("EOF: ", err.Error())
					break
				} else if errors.Is(err, astiav.ErrEagain) {
					break
				}
				opusAudio = append(opusAudio, &MediaPacket{
					Data:       pkt.Data(),
					PTS:        pkt.Pts(),
					DTS:        pkt.Dts(),
					SampleRate: t.encCodecContext.SampleRate(),
				})
			}
		}
	}
	select {
	case t.ResultChan() <- opusAudio:
	default:
	}
	return opusAudio, nil
}
