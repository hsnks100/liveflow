package whep

import (
	"context"
	"errors"
	"fmt"
	"liveflow/media/streamer/processes"

	astiav "github.com/asticode/go-astiav"

	"github.com/deepch/vdk/codec/aacparser"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v3"
	"github.com/sirupsen/logrus"

	"liveflow/log"
	"liveflow/media/hub"
	"liveflow/media/streamer/fields"
)

var (
	ErrNotContainAudioOrVideo = errors.New("media spec does not contain audio or video")
	ErrUnsupportedCodec       = errors.New("unsupported codec")
)

const (
	audioSampleRate = 48000
)

type WHEPArgs struct {
	Tracks map[string][]*webrtc.TrackLocalStaticRTP
	Hub    *hub.Hub
}
type packetWithTimestamp struct {
	packet    *rtp.Packet
	timestamp uint32
}

// whip
type WHEP struct {
	hub                *hub.Hub
	tracks             map[string][]*webrtc.TrackLocalStaticRTP
	audioTrack         *webrtc.TrackLocalStaticRTP
	videoTrack         *webrtc.TrackLocalStaticRTP
	audioPacketizer    rtp.Packetizer
	videoPacketizer    rtp.Packetizer
	lastAudioTimestamp int64
	lastVideoTimestamp int64

	videoBuffer []*packetWithTimestamp
	audioBuffer []*packetWithTimestamp
}

func NewWHEP(args WHEPArgs) *WHEP {
	return &WHEP{
		hub:    args.Hub,
		tracks: args.Tracks,
	}
}

func (w *WHEP) Start(ctx context.Context, source hub.Source) error {
	if !hub.HasCodecType(source.MediaSpecs(), hub.CodecTypeOpus) && !hub.HasCodecType(source.MediaSpecs(), hub.CodecTypeAAC) {
		return ErrUnsupportedCodec
	}
	if !hub.HasCodecType(source.MediaSpecs(), hub.CodecTypeH264) {
		return ErrUnsupportedCodec
	}
	ctx = log.WithFields(ctx, logrus.Fields{
		fields.StreamID:   source.StreamID(),
		fields.SourceName: source.Name(),
	})
	log.Info(ctx, "start whep")
	sub := w.hub.Subscribe(source.StreamID())
	aProcess := processes.NewTranscodingProcess(astiav.CodecIDAac, astiav.CodecIDOpus, audioSampleRate)
	aProcess.Init()
	go func() {
		for data := range sub {
			if data.H264Video != nil {
				w.onVideo(source, data.H264Video)
			}
			if data.AACAudio != nil {
				w.onAACAudio(ctx, source, data.AACAudio, aProcess)
			} else {
				if data.OPUSAudio != nil {
					w.onAudio(source, data.OPUSAudio)
				}
			}
		}
	}()
	return nil
}

func (w *WHEP) onVideo(source hub.Source, h264Video *hub.H264Video) {
	if w.videoTrack == nil {
		var err error
		w.videoTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "pion")
		if err != nil {
			panic(err)
		}
		w.tracks[source.StreamID()] = append(w.tracks[source.StreamID()], w.videoTrack)
		ssrc := uint32(110)
		const (
			h264PayloadType = 96
			mtu             = 1400
		)
		w.videoPacketizer = rtp.NewPacketizer(mtu, h264PayloadType, ssrc, &codecs.H264Payloader{}, rtp.NewRandomSequencer(), h264Video.VideoClockRate)
	}

	videoDuration := h264Video.DTS - w.lastVideoTimestamp
	videoPackets := w.videoPacketizer.Packetize(h264Video.Data, uint32(videoDuration))

	for _, packet := range videoPackets {
		w.videoBuffer = append(w.videoBuffer, &packetWithTimestamp{packet: packet, timestamp: uint32(h264Video.RawDTS())})
	}

	w.lastVideoTimestamp = h264Video.DTS
	w.syncAndSendPackets()
}

func (w *WHEP) onAudio(source hub.Source, opusAudio *hub.OPUSAudio) {
	if w.audioTrack == nil {
		var err error
		w.audioTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion")
		if err != nil {
			panic(err)
		}
		w.tracks[source.StreamID()] = append(w.tracks[source.StreamID()], w.audioTrack)
		ssrc := uint32(111)
		const (
			opusPayloadType = 111
			mtu             = 1400
		)
		w.audioPacketizer = rtp.NewPacketizer(mtu, opusPayloadType, ssrc, &codecs.OpusPayloader{}, rtp.NewRandomSequencer(), opusAudio.AudioClockRate)
	}

	audioDuration := opusAudio.DTS - w.lastAudioTimestamp
	audioPackets := w.audioPacketizer.Packetize(opusAudio.Data, uint32(audioDuration))

	for _, packet := range audioPackets {
		w.audioBuffer = append(w.audioBuffer, &packetWithTimestamp{packet: packet, timestamp: uint32(opusAudio.RawDTS())})
	}

	w.lastAudioTimestamp = opusAudio.DTS
	w.syncAndSendPackets()
}

func (w *WHEP) syncAndSendPackets() {
	for len(w.videoBuffer) > 0 && len(w.audioBuffer) > 0 {
		videoPacket := w.videoBuffer[0]
		audioPacket := w.audioBuffer[0]
		// 뒤쳐지는 패킷을 버퍼에서 제거
		if videoPacket.timestamp <= audioPacket.timestamp {
			// 오디오가 더 빠르면 비디오를 버퍼에서 제거
			w.videoBuffer = w.videoBuffer[1:]
			if err := w.videoTrack.WriteRTP(videoPacket.packet); err != nil {
				panic(err)
			}
		} else {
			// 비디오가 더 빠르면 오디오를 버퍼에서 제거
			w.audioBuffer = w.audioBuffer[1:]
			if err := w.audioTrack.WriteRTP(audioPacket.packet); err != nil {
				panic(err)
			}
		}
	}
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
func (w *WHEP) onAACAudio(ctx context.Context, source hub.Source, aac *hub.AACAudio, transcodingProcess *processes.AudioTranscodingProcess) {
	if len(aac.Data) == 0 {
		fmt.Println("no data")
		return
	}
	if aac.MPEG4AudioConfig == nil {
		fmt.Println("no config")
		return
	}
	const (
		aacSamples     = 1024
		adtsHeaderSize = 7
	)
	adtsHeader := make([]byte, adtsHeaderSize)

	aacparser.FillADTSHeader(adtsHeader, *aac.MPEG4AudioConfig, aacSamples, len(aac.Data))
	audioDataWithADTS := append(adtsHeader, aac.Data...)
	packets, err := transcodingProcess.Process(&processes.MediaPacket{
		Data: audioDataWithADTS,
		PTS:  aac.PTS,
		DTS:  aac.DTS,
	})
	if err != nil {
		fmt.Println(err)
	}
	for _, packet := range packets {
		w.onAudio(source, &hub.OPUSAudio{
			Data:           packet.Data,
			PTS:            packet.PTS,
			DTS:            packet.DTS,
			AudioClockRate: uint32(packet.SampleRate),
		})
	}
}
