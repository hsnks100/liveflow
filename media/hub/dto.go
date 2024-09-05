package hub

import (
	"github.com/deepch/vdk/codec/aacparser"
)

type FrameData struct {
	H264Video *H264Video
	AACAudio  *AACAudio
	OPUSAudio *OPUSAudio
	//AudioCodecData *AudioCodecData
	//MediaInfo      *MediaInfo
}

type H264Video struct {
	PTS            int64
	DTS            int64
	VideoClockRate uint32
	Data           []byte
	SPS            []byte
	PPS            []byte
	SliceType      SliceType
	CodecData      []byte
}

func (h *H264Video) RawTimestamp() int64 {
	if h.VideoClockRate == 0 {
		return h.PTS
	} else {
		return int64(float64(h.PTS) / float64(h.VideoClockRate) * 1000)
	}
}

func (h *H264Video) RawPTS() int64 {
	if h.VideoClockRate == 0 {
		return h.PTS
	} else {
		return int64(float64(h.PTS) / float64(h.VideoClockRate/1000.0))
	}
}
func (h *H264Video) RawDTS() int64 {
	if h.VideoClockRate == 0 {
		return h.DTS
	} else {
		return int64(float64(h.DTS) / float64(h.VideoClockRate/1000.0))
	}
}

type OPUSAudio struct {
	PTS            int64
	DTS            int64
	AudioClockRate uint32
	Data           []byte
}

func (a *OPUSAudio) RawTimestamp() int64 {
	if a.AudioClockRate == 0 {
		return a.PTS
	} else {
		return int64(float64(a.PTS) / float64(a.AudioClockRate) * 1000)
	}
}

func (a *OPUSAudio) RawPTS() int64 {
	if a.AudioClockRate == 0 {
		return a.PTS
	} else {
		return int64(float64(a.PTS) / float64(a.AudioClockRate/1000.0))
	}
}

func (a *OPUSAudio) RawDTS() int64 {
	if a.AudioClockRate == 0 {
		return a.DTS
	} else {
		return int64(float64(a.DTS) / float64(a.AudioClockRate/1000.0))
	}
}

type AACAudio struct {
	Data                  []byte
	SequenceHeader        bool
	MPEG4AudioConfigBytes []byte
	MPEG4AudioConfig      *aacparser.MPEG4AudioConfig
	PTS                   int64
	DTS                   int64
	AudioClockRate        uint32
}

func (a *AACAudio) RawTimestamp() int64 {
	if a.AudioClockRate == 0 {
		return a.PTS
	} else {
		return int64(float64(a.PTS) / float64(a.AudioClockRate) * 1000)
	}
}

func (a *AACAudio) RawPTS() int64 {
	if a.AudioClockRate == 0 {
		return a.PTS
	} else {
		return int64(float64(a.PTS) / float64(a.AudioClockRate/1000.0))
	}
}

func (a *AACAudio) RawDTS() int64 {
	if a.AudioClockRate == 0 {
		return a.DTS
	} else {
		return int64(float64(a.DTS) / float64(a.AudioClockRate/1000.0))
	}
}

type AudioCodecData struct {
	Timestamp uint32
	Data      []byte
}

type VideoCodecData struct {
	Timestamp uint32
	Data      []byte
}
type MetaData struct {
	Timestamp uint32
	Data      []byte
}

type MediaInfo struct {
	VCodec VideoCodecType
}

type VideoCodecType int

const (
	H264 VideoCodecType = iota
	VP8
	// Add other codecs as needed
)
