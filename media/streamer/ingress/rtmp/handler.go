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

	"liveflow/log"
	"liveflow/media/hub"
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

	mediaSpecs     []hub.MediaSpec
	notifiedSource bool
}

func (h *Handler) Depth() int {
	return 0
}

func (h *Handler) Name() string {
	return "rtmp"
}

func (h *Handler) MediaSpecs() []hub.MediaSpec {
	return h.mediaSpecs
}

func (h *Handler) StreamID() string {
	return h.streamID
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
	ctx := context.Background()
	log.Infof(ctx, "OnPublish: %#v", cmd)

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
	h.mediaSpecs = []hub.MediaSpec{
		{
			MediaType: hub.Video,
			ClockRate: 90000,
			CodecType: "h264",
		},
		{
			MediaType: hub.Audio,
			ClockRate: aacDefaultSampleRate,
			CodecType: "aac",
		},
	}

	if !h.notifiedSource && len(h.mediaSpecs) == 2 {
		h.hub.Notify(ctx, h)
		h.notifiedSource = true
	}
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
			DTS:            int64(float64(timestamp) * (audioClockRate / 1000.0)),
			PTS:            int64(float64(timestamp) * (audioClockRate / 1000.0)),
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
	h.hub.Publish(h.streamID, &frameData)
	return nil
}

func (h *Handler) OnVideo(timestamp uint32, payload io.Reader) error {
	ctx := context.Background()

	// payload 데이터를 버퍼에 읽어오기
	payloadBuffer, err := h.readPayload(payload)
	if err != nil {
		log.Error(ctx, err, "Failed to read video payload")
		return err
	}

	// VideoData 구조체로 디코딩
	videoData, err := h.decodeVideoData(payloadBuffer)
	if err != nil {
		return err
	}
	if videoData.CodecID == flvtag.CodecIDAVC {
	}

	// FLV 바디 데이터를 처리하고 대응하는 작업 수행
	return h.processVideoData(ctx, timestamp, videoData)
}

// payload 데이터를 읽어와서 버퍼에 저장하는 함수
func (h *Handler) readPayload(payload io.Reader) (*bytes.Buffer, error) {
	var payloadBuffer bytes.Buffer
	_, err := io.Copy(&payloadBuffer, payload)
	if err != nil {
		return nil, err
	}
	return &payloadBuffer, nil
}

// payload 데이터를 VideoData 구조체로 디코딩하는 함수
func (h *Handler) decodeVideoData(payloadBuffer *bytes.Buffer) (*flvtag.VideoData, error) {
	var videoData flvtag.VideoData
	err := flvtag.DecodeVideoData(bytes.NewBuffer(payloadBuffer.Bytes()), &videoData)
	if err != nil {
		return nil, err
	}
	return &videoData, nil
}

// VideoData에 따라 데이터를 처리하는 함수
func (h *Handler) processVideoData(ctx context.Context, timestamp uint32, videoData *flvtag.VideoData) error {
	flvBodyBuffer := new(bytes.Buffer)
	if _, err := io.Copy(flvBodyBuffer, videoData.Data); err != nil {
		return err
	}
	videoData.Data = flvBodyBuffer

	switch videoData.AVCPacketType {
	case flvtag.AVCPacketTypeSequenceHeader:
		return h.handleSequenceHeader(ctx, flvBodyBuffer)

	case flvtag.AVCPacketTypeNALU:
		return h.handleNALU(ctx, timestamp, videoData.CompositionTime, flvBodyBuffer)
	}

	return nil
}

// AVCPacketTypeSequenceHeader를 처리하는 함수
func (h *Handler) handleSequenceHeader(ctx context.Context, flvBodyBuffer *bytes.Buffer) error {
	seqHeader, err := h264parser.NewCodecDataFromAVCDecoderConfRecord(flvBodyBuffer.Bytes())
	if err != nil {
		log.Error(ctx, err, "Failed to parse AVCDecoderConfigurationRecord")
		return err
	}

	h.width = seqHeader.Width()
	h.height = seqHeader.Height()
	h.sps = append([]byte{}, seqHeader.SPS()...)
	h.pps = append([]byte{}, seqHeader.PPS()...)
	h.hasSPS = true

	log.Info(ctx, "Received AVCPacketTypeSequenceHeader")
	return nil
}

// AVCPacketTypeNALU를 처리하는 함수
func (h *Handler) handleNALU(ctx context.Context, timestamp uint32, compositionTime int32, flvBodyBuffer *bytes.Buffer) error {
	h.updateSPSPPS(flvBodyBuffer.Bytes())

	videoDataToSend := h.prepareVideoData(flvBodyBuffer.Bytes())
	if len(videoDataToSend) == 0 {
		return nil
	}

	h.publishVideoData(timestamp, compositionTime, videoDataToSend)
	return nil
}

// NALU 데이터를 분석하여 SPS, PPS 정보를 업데이트하는 함수
func (h *Handler) updateSPSPPS(naluData []byte) {
	nalus, _ := h264parser.SplitNALUs(naluData)
	for _, nalu := range nalus {
		if len(nalu) < 1 {
			continue
		}
		nalUnitType := nalu[0] & 0x1f
		switch nalUnitType {
		case h264parser.NALU_SPS:
			h.sps = append([]byte{}, nalu...)
		case h264parser.NALU_PPS:
			h.pps = append([]byte{}, nalu...)
		}
	}
}

// NALU 데이터를 준비하여 전송할 비디오 데이터를 생성하는 함수
func (h *Handler) prepareVideoData(naluData []byte) []byte {
	var videoDataToSend []byte
	hasSPSInData := false
	startCode := []byte{0, 0, 0, 1}

	nalus, _ := h264parser.SplitNALUs(naluData)
	for _, nalu := range nalus {
		if len(nalu) < 1 {
			continue
		}
		sliceType, _ := h264parser.ParseSliceHeaderFromNALU(nalu)
		nalUnitType := nalu[0] & 0x1f
		switch nalUnitType {
		case h264parser.NALU_SPS, h264parser.NALU_PPS:
			// SPS와 PPS는 이미 처리됨
		default:
			// I-프레임일 때 SPS, PPS 추가
			if sliceType == h264parser.SLICE_I && !hasSPSInData {
				videoDataToSend = append(videoDataToSend, startCode...)
				videoDataToSend = append(videoDataToSend, h.sps...)
				videoDataToSend = append(videoDataToSend, startCode...)
				videoDataToSend = append(videoDataToSend, h.pps...)
				hasSPSInData = true
			}
			videoDataToSend = append(videoDataToSend, startCode...)
			videoDataToSend = append(videoDataToSend, nalu...)
		}
	}
	return videoDataToSend
}

// 비디오 데이터를 Hub에 전송하는 함수
func (h *Handler) publishVideoData(timestamp uint32, compositionTime int32, videoDataToSend []byte) {
	dts := int64(timestamp)
	pts := int64(compositionTime) + dts

	h.hub.Publish(h.streamID, &hub.FrameData{
		H264Video: &hub.H264Video{
			VideoClockRate: 90000,
			DTS:            dts * 90,
			PTS:            pts * 90,
			Data:           videoDataToSend,
			SPS:            h.sps,
			PPS:            h.pps,
			CodecData:      nil,
		},
	})
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
