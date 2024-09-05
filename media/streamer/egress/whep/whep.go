package whep

import (
	"context"
	"errors"
	"fmt"
	astiav "liveflow/goastiav"
	"liveflow/media/streamer/processes"

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
	containsAudio := false
	containsVideo := false
	audioCodec := ""
	videoCodec := ""
	for _, spec := range source.MediaSpecs() {
		if spec.MediaType == hub.Audio {
			containsAudio = true
			audioCodec = spec.CodecType
		}
		if spec.MediaType == hub.Video {
			containsVideo = true
			videoCodec = spec.CodecType
		}
	}
	if !containsVideo || !containsAudio {
		return ErrNotContainAudioOrVideo
	}
	fmt.Println("audioCodec", audioCodec)
	fmt.Println("videoCodec", videoCodec)
	if audioCodec != "opus" && audioCodec != "aac" {
		return fmt.Errorf("%w: %s", ErrUnsupportedCodec, audioCodec)
	}
	if videoCodec != "h264" {
		return fmt.Errorf("%w: %s", ErrUnsupportedCodec, videoCodec)
	}
	ctx = log.WithFields(ctx, logrus.Fields{
		fields.StreamID:   source.StreamID(),
		fields.SourceName: source.Name(),
	})
	log.Info(ctx, "start whep")
	sub := w.hub.Subscribe(source.StreamID())
	//resultChan := aProcess.ResultChan()
	//bProcess := processes.NewDumpProcess()
	//bProcess.SetTimeout(500 * time.Millisecond)
	//pipe.LinkProcesses[hub.H264Video, []*astiav.Frame, interface{}](aProcess, bProcess)
	//starter := pipe.MakeStarter(aProcess)
	//pipeExecutor := pipe.NewPipeExecutor[*processes.MediaPacket, []*processes.MediaPacket](aProcess, 5000*time.Millisecond)
	aProcess := processes.NewTranscodingProcess(astiav.CodecIDAac, astiav.CodecIDOpus)
	aProcess.Init()
	go func() {
		for data := range sub {
			if data.H264Video != nil {
				w.OnVideo(source, data.H264Video)
			}
			if data.AACAudio != nil {
				w.OnAACAudio(ctx, source, data.AACAudio, aProcess)
			} else {
				if data.OPUSAudio != nil {
					w.OnAudio(source, data.OPUSAudio)
				}
			}
		}
	}()
	return nil
}

func (w *WHEP) OnVideo(source hub.Source, h264Video *hub.H264Video) {
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
	duration := h264Video.DTS - w.lastVideoTimestamp
	packets := w.videoPacketizer.Packetize(h264Video.Data, uint32(duration))
	for _, p := range packets {
		if err := w.videoTrack.WriteRTP(p); err != nil {
			panic(err)
		}
	}
	w.lastVideoTimestamp = h264Video.DTS
}

func (w *WHEP) OnAudio(source hub.Source, opusAudio *hub.OPUSAudio) {
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
	// TODO: /whep 요청 들어오면 거기서 트랙만들고 센더 만들고 등록해주기
	duration := opusAudio.DTS - w.lastAudioTimestamp
	packets := w.audioPacketizer.Packetize(opusAudio.Data, uint32(duration))
	for _, p := range packets {
		if err := w.audioTrack.WriteRTP(p); err != nil {
			panic(err)
		}
	}
	w.lastAudioTimestamp = opusAudio.DTS
}

func (w *WHEP) OnAACAudio(ctx context.Context, source hub.Source, aac *hub.AACAudio, transcodingProcess *processes.AudioTranscodingProcess) {
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
		w.OnAudio(source, &hub.OPUSAudio{
			Data:           packet.Data,
			PTS:            packet.PTS,
			DTS:            packet.DTS,
			AudioClockRate: 48000,
		})
	}
}
