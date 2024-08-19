package rtmp

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/deepch/vdk/codec/h264parser"
	"github.com/pkg/errors"
	"github.com/yutopp/go-flv"
	flvtag "github.com/yutopp/go-flv/tag"
	"github.com/yutopp/go-rtmp"
	rtmpmsg "github.com/yutopp/go-rtmp/message"

	"mrw-clone/log"
	"mrw-clone/media/hub"
	"mrw-clone/media/streamer"
)

type Handler struct {
	hub      *hub.Hub
	streamID string
	rtmp.DefaultHandler
	flvFile *os.File
	flvEnc  *flv.Encoder

	width  int
	height int
	sps    []byte
	pps    []byte
	hasSPS bool
}

func (h *Handler) OnServe(conn *rtmp.Conn) {
}

func (h *Handler) OnConnect(timestamp uint32, cmd *rtmpmsg.NetConnectionConnect) error {
	log.Infof(context.Background(), "OnConnect: %#v", cmd)
	return nil
}

func (h *Handler) OnCreateStream(timestamp uint32, cmd *rtmpmsg.NetConnectionCreateStream) error {
	log.Infof(context.Background(), "OnCreateStream: %#v", cmd)
	return nil
}

func (h *Handler) OnPublish(_ *rtmp.StreamContext, timestamp uint32, cmd *rtmpmsg.NetStreamPublish) error {
	log.Infof(context.Background(), "OnPublish: %#v", cmd)

	// (example) Reject a connection when PublishingName is empty
	if cmd.PublishingName == "" {
		return errors.New("PublishingName is empty")
	}

	// Record streams as FLV!
	p := filepath.Join(
		os.TempDir(),
		filepath.Clean(filepath.Join("/", fmt.Sprintf("%s.flv", cmd.PublishingName))),
	)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return errors.Wrap(err, "Failed to create flv file")
	}
	h.flvFile = f

	enc, err := flv.NewEncoder(f, flv.FlagsAudio|flv.FlagsVideo)
	if err != nil {
		_ = f.Close()
		return errors.Wrap(err, "Failed to create flv encoder")
	}
	h.flvEnc = enc

	h.streamID = cmd.PublishingName
	h.hub.Notify(cmd.PublishingName)

	return nil
}

func (h *Handler) OnSetDataFrame(timestamp uint32, data *rtmpmsg.NetStreamSetDataFrame) error {
	r := bytes.NewReader(data.Payload)

	var script flvtag.ScriptData
	if err := flvtag.DecodeScriptData(r, &script); err != nil {
		log.Infof(context.Background(), "Failed to decode script data: Err = %+v", err)
		return nil // ignore
	}

	log.Infof(context.Background(), "SetDataFrame: Script = %#v", script)

	if err := h.flvEnc.Encode(&flvtag.FlvTag{
		TagType:   flvtag.TagTypeScriptData,
		Timestamp: timestamp,
		Data:      &script,
	}); err != nil {
		log.Infof(context.Background(), "Failed to write script data: Err = %+v", err)
	}

	return nil
}

func (h *Handler) OnAudio(timestamp uint32, payload io.Reader) error {
	ctx := context.Background()
	var buf bytes.Buffer
	_, err := io.Copy(&buf, payload)
	if err != nil {
		log.Error(ctx, err, "failed to read audio")
		return err
	}
	var audio flvtag.AudioData
	if err := flvtag.DecodeAudioData(bytes.NewBuffer(buf.Bytes()), &audio); err != nil {
		return err
	}

	flvBody := new(bytes.Buffer)
	if _, err := io.Copy(flvBody, audio.Data); err != nil {
		return err
	}
	audio.Data = flvBody

	frameData := hub.FrameData{
		AACAudio: &hub.AACAudio{
			AudioClockRate: flvSampleRate(audio.SoundRate),
		},
	}
	switch audio.AACPacketType {
	case flvtag.AACPacketTypeSequenceHeader:
		frameData.AACAudio.CodecData = flvBody.Bytes()
		log.Infof(ctx, "AACAudio Sequence Header: %s", hex.Dump(flvBody.Bytes()))
	case flvtag.AACPacketTypeRaw:
		frameData.AACAudio = &hub.AACAudio{
			Timestamp: timestamp,
			Data:      flvBody.Bytes(),
		}
	}
	h.hub.Publish(h.streamID, frameData)
	return nil
}

func (h *Handler) OnVideo(timestamp uint32, payload io.Reader) error {
	ctx := context.Background()
	var buf bytes.Buffer
	_, err := io.Copy(&buf, payload)
	if err != nil {
		log.Error(ctx, err, "failed to read audio")
		return err
	}
	var video flvtag.VideoData
	if err := flvtag.DecodeVideoData(bytes.NewBuffer(buf.Bytes()), &video); err != nil {
		return err
	}

	flvBody := new(bytes.Buffer)
	if _, err := io.Copy(flvBody, video.Data); err != nil {
		return err
	}
	video.Data = flvBody
	switch video.AVCPacketType {
	case flvtag.AVCPacketTypeSequenceHeader:
		log.Info(ctx, "Received AVCPacketTypeSequenceHeader")
		seqHeader, err := h264parser.NewCodecDataFromAVCDecoderConfRecord(flvBody.Bytes())
		if err != nil {
			log.Error(ctx, err, "Failed to NewCodecDataFromAVCDecoderConfRecord")
		} else {
			h.width = seqHeader.Width()
			h.height = seqHeader.Height()
			h.sps = make([]byte, len(seqHeader.SPS()))
			copy(h.sps, seqHeader.SPS())
			h.pps = make([]byte, len(seqHeader.PPS()))
			copy(h.pps, seqHeader.PPS())
		}
		h.hasSPS = true
		return nil
	case flvtag.AVCPacketTypeNALU:
		annexB := []byte{0, 0, 0, 1}
		nals, _ := h264parser.SplitNALUs(flvBody.Bytes())
		for _, n := range nals {
			sliceType, _ := h264parser.ParseSliceHeaderFromNALU(n)
			dts := int64(timestamp)
			pts := int64(video.CompositionTime) + dts
			var hubSliceType hub.SliceType
			switch sliceType {
			case h264parser.SLICE_I:
				hubSliceType = hub.SliceI
			case h264parser.SLICE_P:
				hubSliceType = hub.SliceP
			case h264parser.SLICE_B:
				hubSliceType = hub.SliceB
			}
			h.hub.Publish(h.streamID, hub.FrameData{
				H264Video: &hub.H264Video{
					VideoClockRate: 90000,
					DTS:            dts,
					PTS:            pts,
					Data:           streamer.ConcatByteSlices(annexB, n),
					SPS:            h.sps,
					PPS:            h.pps,
					SliceType:      hubSliceType,
					CodecData:      nil,
				},
			})
		}
		//sliceTypes := parsers.ParseH264Payload(annexBs)
		// chunkmessage 의 timestamp 는 dts 임
	}
	//////////////////////////
	return nil
}

func (h *Handler) OnClose() {
	log.Infof(context.Background(), "OnClose")

	if h.flvFile != nil {
		_ = h.flvFile.Close()
	}
	h.hub.Unpublish(h.streamID)
}

func flvSampleRate(soundRate flvtag.SoundRate) uint32 {
	switch soundRate {
	case flvtag.SoundRate5_5kHz:
		return 5500
	case flvtag.SoundRate11kHz:
		return 11000
	case flvtag.SoundRate22kHz:
		return 22000
	case flvtag.SoundRate44kHz:
		return 44000
	default:
		return aacDefaultSampleRate
	}
}
