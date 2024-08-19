package hub

type FrameData struct {
	H264Video *H264Video
	AACAudio  *AACAudio
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
		return h.PTS / int64(h.VideoClockRate)
	}
}

type AACAudio struct {
	Timestamp      uint32
	Data           []byte
	CodecData      []byte
	AudioClockRate uint32
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
