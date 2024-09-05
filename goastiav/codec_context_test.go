package astiav

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodecContext(t *testing.T) {
	fc, err := globalHelper.inputFormatContext("video.mp4")
	require.NoError(t, err)
	ss := fc.Streams()
	require.Len(t, ss, 2)
	s1 := ss[0]
	s2 := ss[1]

	c1 := FindDecoder(s1.CodecParameters().CodecID())
	require.NotNil(t, c1)
	cc1 := AllocCodecContext(c1)
	require.NotNil(t, cc1)
	defer cc1.Free()
	err = s1.CodecParameters().ToCodecContext(cc1)
	require.NoError(t, err)
	require.Equal(t, "Video: h264 (Constrained Baseline) (avc1 / 0x31637661), yuv420p(progressive), 320x180 [SAR 1:1 DAR 16:9], 441 kb/s", cc1.String())
	require.Equal(t, int64(441324), cc1.BitRate())
	require.Equal(t, ChromaLocationLeft, cc1.ChromaLocation())
	require.Equal(t, CodecIDH264, cc1.CodecID())
	require.Equal(t, ColorPrimariesUnspecified, cc1.ColorPrimaries())
	require.Equal(t, ColorRangeUnspecified, cc1.ColorRange())
	require.Equal(t, ColorSpaceUnspecified, cc1.ColorSpace())
	require.Equal(t, ColorTransferCharacteristicUnspecified, cc1.ColorTransferCharacteristic())
	require.Equal(t, 12, cc1.GopSize())
	require.Equal(t, 180, cc1.Height())
	require.Equal(t, Level(13), cc1.Level())
	require.Equal(t, MediaTypeVideo, cc1.MediaType())
	require.Equal(t, PixelFormatYuv420P, cc1.PixelFormat())
	require.Equal(t, ProfileH264ConstrainedBaseline, cc1.Profile())
	require.Equal(t, NewRational(1, 1), cc1.SampleAspectRatio())
	require.Equal(t, StrictStdComplianceNormal, cc1.StrictStdCompliance())
	require.Equal(t, 1, cc1.ThreadCount())
	require.Equal(t, ThreadType(3), cc1.ThreadType())
	require.Equal(t, 320, cc1.Width())
	cl := cc1.Class()
	require.NotNil(t, cl)
	require.Equal(t, "AVCodecContext", cl.Name())

	c2 := FindDecoder(s2.CodecParameters().CodecID())
	require.NotNil(t, c2)
	cc2 := AllocCodecContext(c2)
	require.NotNil(t, cc2)
	defer cc2.Free()
	err = s2.CodecParameters().ToCodecContext(cc2)
	require.NoError(t, err)
	require.Equal(t, "Audio: aac (LC) (mp4a / 0x6134706D), 48000 Hz, stereo, fltp, 161 kb/s", cc2.String())
	require.Equal(t, int64(161052), cc2.BitRate())
	require.True(t, cc2.ChannelLayout().Equal(ChannelLayoutStereo))
	require.Equal(t, CodecIDAac, cc2.CodecID())
	require.Equal(t, 1024, cc2.FrameSize())
	require.Equal(t, MediaTypeAudio, cc2.MediaType())
	require.Equal(t, SampleFormatFltp, cc2.SampleFormat())
	require.Equal(t, 48000, cc2.SampleRate())
	require.Equal(t, StrictStdComplianceNormal, cc2.StrictStdCompliance())
	require.Equal(t, 1, cc2.ThreadCount())
	require.Equal(t, ThreadType(3), cc2.ThreadType())

	c3 := FindEncoder(CodecIDMjpeg)
	require.NotNil(t, c3)
	cc3 := AllocCodecContext(c3)
	require.NotNil(t, cc3)
	defer cc3.Free()
	cc3.SetHeight(2)
	cc3.SetPixelFormat(PixelFormatYuvj420P)
	cc3.SetTimeBase(NewRational(1, 1))
	cc3.SetWidth(3)
	err = cc3.Open(c3, nil)
	require.NoError(t, err)

	cc4 := AllocCodecContext(nil)
	require.NotNil(t, cc4)
	defer cc4.Free()
	cc4.SetBitRate(1)
	cc4.SetChannelLayout(ChannelLayout21)
	cc4.SetFlags(NewCodecContextFlags(4))
	cc4.SetFlags2(NewCodecContextFlags2(5))
	cc4.SetFramerate(NewRational(6, 1))
	cc4.SetGopSize(7)
	cc4.SetHeight(8)
	cc4.SetLevel(16)
	cc4.SetProfile(ProfileH264Extended)
	cc4.SetPixelFormat(PixelFormat0Bgr)
	cc4.SetQmin(5)
	cc4.SetSampleAspectRatio(NewRational(10, 1))
	cc4.SetSampleFormat(SampleFormatDbl)
	cc4.SetSampleRate(12)
	cc4.SetStrictStdCompliance(StrictStdComplianceExperimental)
	cc4.SetThreadCount(13)
	cc4.SetThreadType(ThreadTypeSlice)
	cc4.SetTimeBase(NewRational(15, 1))
	cc4.SetWidth(16)
	cc4.SetExtraHardwareFrames(4)
	require.Equal(t, int64(1), cc4.BitRate())
	require.True(t, cc4.ChannelLayout().Equal(ChannelLayout21))
	require.Equal(t, NewCodecContextFlags(4), cc4.Flags())
	require.Equal(t, NewCodecContextFlags2(5), cc4.Flags2())
	require.Equal(t, NewRational(6, 1), cc4.Framerate())
	require.Equal(t, 7, cc4.GopSize())
	require.Equal(t, 8, cc4.Height())
	require.Equal(t, Level(16), cc4.Level())
	require.Equal(t, ProfileH264Extended, cc4.Profile())
	require.Equal(t, PixelFormat0Bgr, cc4.PixelFormat())
	require.Equal(t, 5, cc4.Qmin())
	require.Equal(t, NewRational(10, 1), cc4.SampleAspectRatio())
	require.Equal(t, SampleFormatDbl, cc4.SampleFormat())
	require.Equal(t, 12, cc4.SampleRate())
	require.Equal(t, StrictStdComplianceExperimental, cc4.StrictStdCompliance())
	require.Equal(t, 13, cc4.ThreadCount())
	require.Equal(t, ThreadTypeSlice, cc4.ThreadType())
	require.Equal(t, NewRational(15, 1), cc4.TimeBase())
	require.Equal(t, 16, cc4.Width())
	require.Equal(t, 4, cc4.ExtraHardwareFrames())

	cc5 := AllocCodecContext(nil)
	require.NotNil(t, cc5)
	defer cc5.Free()
	err = cc5.FromCodecParameters(s2.CodecParameters())
	require.NoError(t, err)
	require.Equal(t, s2.CodecParameters().CodecID(), cc5.CodecID())

	cp1 := AllocCodecParameters()
	require.NotNil(t, cp1)
	defer cp1.Free()
	err = cc5.ToCodecParameters(cp1)
	require.NoError(t, err)
	require.Equal(t, cc5.CodecID(), cp1.CodecID())

	cc6 := AllocCodecContext(nil)
	require.NotNil(t, cc6)
	b := []byte("test")
	require.NoError(t, cc6.SetExtraData(b))
	require.Equal(t, b, cc6.ExtraData())

	// TODO Test ReceivePacket
	// TODO Test SendPacket
	// TODO Test ReceiveFrame
	// TODO Test SendFrame
}
