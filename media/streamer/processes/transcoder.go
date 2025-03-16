package processes

import (
	"context"
	"errors"
	"fmt"

	astiav "github.com/asticode/go-astiav"

	"liveflow/log"
)

type MediaPacket struct {
	Data       []byte
	PTS        int64
	DTS        int64
	SampleRate int
}
type AudioTranscodingProcess struct {
	//pipe.BaseProcess[*MediaPacket, []*MediaPacket]
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
	return nil
}

func (t *AudioTranscodingProcess) Close() {
	if t.decCodecContext != nil {
		t.decCodecContext.Free()
	}
	if t.encCodecContext != nil {
		t.encCodecContext.Free()
	}
	if t.audioFifo != nil {
		t.audioFifo.Free()
	}
}

func (t *AudioTranscodingProcess) Process(data *MediaPacket) ([]*MediaPacket, error) {
	ctx := context.Background()
	frame := astiav.AllocFrame()
	defer frame.Free()
	packet := astiav.AllocPacket()
	defer packet.Free()
	if err := packet.FromData(data.Data); err != nil {
		log.Error(ctx, err, "failed to create packet")
	}
	packet.SetPts(data.PTS)
	packet.SetDts(data.DTS)
	if err := t.decCodecContext.SendPacket(packet); err != nil {
		log.Error(ctx, err, "failed to send packet")
	}

	frameToSend := astiav.AllocFrame()
	defer frameToSend.Free()
	if t.audioFifo == nil {
		t.audioFifo = astiav.AllocAudioFifo(
			t.encCodecContext.SampleFormat(),
			t.encCodecContext.ChannelLayout().Channels(),
			t.encCodecContext.SampleRate(),
		)
	}

	var opusAudio []*MediaPacket

	for {
		err := t.decCodecContext.ReceiveFrame(frame)
		if errors.Is(err, astiav.ErrEof) {
			fmt.Println("EOF: ", err.Error())
			break
		} else if errors.Is(err, astiav.ErrEagain) {
			break
		}
		t.audioFifo.Write(frame)

		// check whether we have enough samples to encode
		for t.audioFifo.Size() >= t.encCodecContext.FrameSize() {
			// initialize frameToSend before reusing it
			frameToSend.Unref()
			frameToSend.SetNbSamples(t.encCodecContext.FrameSize())
			frameToSend.SetChannelLayout(t.encCodecContext.ChannelLayout())
			frameToSend.SetSampleFormat(t.encCodecContext.SampleFormat())
			frameToSend.SetSampleRate(t.encCodecContext.SampleRate())
			frameToSend.SetPts(t.lastPts + int64(t.encCodecContext.FrameSize()))
			if err := frameToSend.AllocBuffer(0); err != nil {
				log.Error(ctx, err, "failed to alloc buffer")
			}
			t.lastPts = frameToSend.Pts()

			read, err := t.audioFifo.Read(frameToSend)
			if err != nil {
				log.Error(ctx, err, "failed to read fifo")
			}
			if read < frameToSend.NbSamples() {
				log.Error(ctx, err, "failed to read fifo")
			}

			if err := t.encCodecContext.SendFrame(frameToSend); err != nil {
				log.Error(ctx, err, "failed to send frame")
			}

			pkt := astiav.AllocPacket()
			for {
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
				pkt.Unref()
			}
			pkt.Free()
		}
		frame.Unref()
	}

	return opusAudio, nil
}
