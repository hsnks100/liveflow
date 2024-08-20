package rtmp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/deepch/vdk/codec/aacparser"
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

	audioClockRate := float64(flvSampleRate(audio.SoundRate))
	frameData := hub.FrameData{
		AACAudio: &hub.AACAudio{
			AudioClockRate: uint32(audioClockRate),
			//DTS:            int64(float64(timestamp) / 1000 * audioClockRate),
			//PTS:            int64(float64(timestamp) / 1000 * audioClockRate),
			DTS: int64(timestamp),
			PTS: int64(timestamp),
		},
	}
	switch audio.AACPacketType {
	case flvtag.AACPacketTypeSequenceHeader:
		//log.Infof(ctx, "AACAudio Sequence Header: %s", hex.Dump(flvBody.Bytes()))
		codecData, err := aacparser.NewCodecDataFromMPEG4AudioConfigBytes(flvBody.Bytes())
		if err != nil {
			log.Error(ctx, err, "failed to NewCodecDataFromMPEG4AudioConfigBytes")
			return err
		}
		frameData.AACAudio.MPEG4AudioConfig = &codecData.Config
		frameData.AACAudio.MPEG4AudioConfigBytes = codecData.MPEG4AudioConfigBytes()
	case flvtag.AACPacketTypeRaw:
		frameData.AACAudio.Data = flvBody.Bytes()
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
		// update SPS PPS
		{
			nals, _ := h264parser.SplitNALUs(flvBody.Bytes())
			for _, n := range nals {
				if len(n) < 1 {
					continue
				}
				nalType := n[0] & 0x1f
				switch nalType {
				case h264parser.NALU_SPS:
					h.sps = make([]byte, len(n))
					copy(h.sps, n)
				case h264parser.NALU_PPS:
					h.pps = make([]byte, len(n))
					copy(h.pps, n)
				}
			}
		}
		// send video
		{
			var sendData []byte
			nals, _ := h264parser.SplitNALUs(flvBody.Bytes())
			//hasSPS := false
			//data := make([]byte, 0)
			startCode := []byte{0, 0, 0, 1}

			for _, n := range nals {
				if len(n) < 1 {
					continue
				}
				//sliceType, _ := h264parser.ParseSliceHeaderFromNALU(n)
				//var hubSliceType hub.SliceType
				nalType := n[0] & 0x1f
				switch nalType {
				case h264parser.NALU_SPS:
					//fmt.Println("SPS")
					//hubSliceType = hub.SliceSPS
					sendData = streamer.ConcatByteSlices(sendData, startCode, n)
				case h264parser.NALU_PPS:
					//fmt.Println("PPS")
					//hubSliceType = hub.SlicePPS
					sendData = streamer.ConcatByteSlices(sendData, startCode, n)
				default:
					sendData = streamer.ConcatByteSlices(sendData, startCode, n)
					//if sliceType == h264parser.SLICE_I || sliceType == h264parser.SLICE_P {
					//	data = streamer.ConcatByteSlices(data, n)
					//} else {
					//	sendData = streamer.ConcatByteSlices(sendData, startCode, n)
					//}
				}
				//nalType := n[0] & 0x1f
				//switch nalType {
				//case h264parser.NALU_SPS:
				//	fmt.Println("SPS")
				//	//hubSliceType = hub.SliceSPS
				//case h264parser.NALU_PPS:
				//	fmt.Println("PPS")
				//	//hubSliceType = hub.SlicePPS
				//default:
				//	//fmt.Println(hex.Dump(h.sps))
				//	//fmt.Println("sliceType: ", sliceType.String(), len(h.sps), len(h.pps), len(n), timestamp)
				//	//fmt.Println(hex.Dump(n[:15]))
				//	if sliceType == h264parser.SLICE_I && !hasSPS {
				//		sendData = streamer.ConcatByteSlices(startCode, h.sps, startCode, h.pps)
				//		hasSPS = true
				//	}
				//}
			}
			//sendData = streamer.ConcatByteSlices(sendData, startCode, data)
			if len(sendData) == 0 {
				return nil
			}
			//fmt.Println("#", hex.Dump(sendData))
			dts := int64(timestamp)
			pts := (int64(video.CompositionTime) + dts)
			h.hub.Publish(h.streamID, hub.FrameData{
				H264Video: &hub.H264Video{
					VideoClockRate: 90000,
					DTS:            dts, // * 90,
					PTS:            pts, // * 90,
					Data:           sendData,
					SPS:            h.sps,
					PPS:            h.pps,
					//SliceType:      hubSliceType,
					CodecData: nil,
				},
			})
		}
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
