package whep

// #include <stdio.h>
// #include <stdlib.h>
//
// void __lsan_do_leak_check(void);
import "C"
import (
	"context"
	"errors"

	"github.com/asticode/go-astiav"

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
	ErrUnsupportedCodec = errors.New("unsupported codec")
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

// WHEP represents a WebRTC to HLS conversion pipeline.
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
	go func() {
		var audioTranscodingProcess *processes.AudioTranscodingProcess
		for data := range sub {
			if data.H264Video != nil {
				err := w.onVideo(source, data.H264Video)
				if err != nil {
					log.Error(ctx, err, "failed to process video")
				}
			}
			if data.AACAudio != nil {
				if audioTranscodingProcess == nil {
					audioTranscodingProcess = processes.NewTranscodingProcess(astiav.CodecIDAac, astiav.CodecIDOpus, audioSampleRate)
					audioTranscodingProcess.Init()
				}
				w.addAudioTrack(ctx, source.StreamID())
				err := w.onAACAudio(ctx, source, data.AACAudio, audioTranscodingProcess)
				if err != nil {
					log.Error(ctx, err, "failed to process AAC audio")
				}
			} else if data.OPUSAudio != nil {
				w.addAudioTrack(ctx, source.StreamID())
				err := w.onAudio(source, data.OPUSAudio)
				if err != nil {
					log.Error(ctx, err, "failed to process OPUS audio")
				}
			}
		}

		if audioTranscodingProcess != nil {
			log.Info(ctx, "draining audio transcoding process for WHEP")
			packets, err := audioTranscodingProcess.Drain()
			if err != nil {
				log.Error(ctx, err, "failed to drain audio transcoder for WHEP")
			}
			for _, packet := range packets {
				err := w.onAudio(source, &hub.OPUSAudio{
					Data:           packet.Data,
					PTS:            packet.PTS,
					DTS:            packet.DTS,
					AudioClockRate: uint32(packet.SampleRate),
				})
				if err != nil {
					log.Error(ctx, err, "failed to process drained OPUS audio for WHEP")
				}
			}
			audioTranscodingProcess.Close()
			log.Info(ctx, "audio transcoding process for WHEP drained and closed")
		}
		log.Info(ctx, "end whep")
		//C.__lsan_do_leak_check()
	}()

	return nil
}

func (w *WHEP) addVideoTrack(streamID string, videoClockRate uint32) error {
	if w.videoTrack == nil {
		var err error
		w.videoTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "pion")
		if err != nil {
			return err
		}
		w.tracks[streamID] = append(w.tracks[streamID], w.videoTrack)
		ssrc := uint32(110)
		const (
			h264PayloadType = 96
			mtu             = 1400
		)
		w.videoPacketizer = rtp.NewPacketizer(mtu, h264PayloadType, ssrc, &codecs.H264Payloader{}, rtp.NewRandomSequencer(), videoClockRate)
	}
	return nil
}

func (w *WHEP) addAudioTrack(ctx context.Context, streamID string) error {
	if w.audioTrack == nil {
		var err error
		w.audioTrack, err = webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "pion")
		if err != nil {
			log.Error(ctx, err, "failed to create audio track")
		}
		w.tracks[streamID] = append(w.tracks[streamID], w.audioTrack)
		ssrc := uint32(111)
		const (
			opusPayloadType = 111
			mtu             = 1400
		)
		w.audioPacketizer = rtp.NewPacketizer(mtu, opusPayloadType, ssrc, &codecs.OpusPayloader{}, rtp.NewRandomSequencer(), 48000)
	}
	return nil
}
func (w *WHEP) onVideo(source hub.Source, h264Video *hub.H264Video) error {
	w.addVideoTrack(source.StreamID(), h264Video.VideoClockRate)

	videoDuration := h264Video.DTS - w.lastVideoTimestamp
	videoPackets := w.videoPacketizer.Packetize(h264Video.Data, uint32(videoDuration))

	for _, packet := range videoPackets {
		w.videoBuffer = append(w.videoBuffer, &packetWithTimestamp{packet: packet, timestamp: uint32(h264Video.RawDTS())})
	}

	w.lastVideoTimestamp = h264Video.DTS
	w.syncAndSendPackets()
	return nil
}

func (w *WHEP) onAudio(source hub.Source, opusAudio *hub.OPUSAudio) error {
	audioDuration := opusAudio.DTS - w.lastAudioTimestamp
	audioPackets := w.audioPacketizer.Packetize(opusAudio.Data, uint32(audioDuration))

	for _, packet := range audioPackets {
		w.audioBuffer = append(w.audioBuffer, &packetWithTimestamp{packet: packet, timestamp: uint32(opusAudio.RawDTS())})
	}

	w.lastAudioTimestamp = opusAudio.DTS
	err := w.syncAndSendPackets()
	if err != nil {
		return err
	}
	return nil
}

func (w *WHEP) onAACAudio(ctx context.Context, source hub.Source, aac *hub.AACAudio, transcodingProcess *processes.AudioTranscodingProcess) error {
	if len(aac.Data) == 0 {
		log.Warn(ctx, "no data")
		return nil
	}
	if aac.MPEG4AudioConfig == nil {
		log.Warn(ctx, "no config")
		return nil
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
		return err
	}
	for _, packet := range packets {
		w.onAudio(source, &hub.OPUSAudio{
			Data:           packet.Data,
			PTS:            packet.PTS,
			DTS:            packet.DTS,
			AudioClockRate: uint32(packet.SampleRate),
		})
	}
	return nil
}

func (w *WHEP) syncAndSendPackets() error {
	for len(w.videoBuffer) > 0 && len(w.audioBuffer) > 0 {
		videoPacket := w.videoBuffer[0]
		audioPacket := w.audioBuffer[0]
		// Remove lagging packet from buffer
		if videoPacket.timestamp <= audioPacket.timestamp {
			// If audio is ahead, remove video from buffer
			w.videoBuffer[0] = nil
			w.videoBuffer = w.videoBuffer[1:]
			if err := w.videoTrack.WriteRTP(videoPacket.packet); err != nil {
				return err
			}
		} else {
			// If video is ahead, remove audio from buffer
			w.audioBuffer[0] = nil
			w.audioBuffer = w.audioBuffer[1:]
			if err := w.audioTrack.WriteRTP(audioPacket.packet); err != nil {
				return err
			}
		}
	}
	return nil
}
