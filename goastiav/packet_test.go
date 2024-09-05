package astiav

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPacket(t *testing.T) {
	pkt1, err := globalHelper.inputFirstPacket("video.mp4")
	require.NoError(t, err)
	require.Equal(t, []byte{0x0, 0x0, 0x0, 0xd1, 0x65, 0x88, 0x82, 0x0, 0x1f, 0x5f, 0xff, 0xf8, 0x22, 0x8a, 0x0, 0x2, 0x2d, 0xbe, 0x38, 0xc7, 0x19, 0x39, 0x39, 0x39, 0x39, 0x39, 0x39, 0x39, 0x39, 0x39, 0x39, 0x39, 0x39, 0x39, 0x39, 0x39, 0x39, 0x39, 0x39, 0x3a, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xba, 0xeb, 0xae, 0xb9, 0xb8, 0xe6, 0x39, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xc0}, pkt1.Data())
	require.Equal(t, int64(0), pkt1.Dts())
	require.Equal(t, int64(512), pkt1.Duration())
	require.True(t, pkt1.Flags().Has(PacketFlagKey))
	require.Equal(t, int64(48), pkt1.Pos())
	require.Equal(t, int64(0), pkt1.Pts())
	require.Equal(t, 213, pkt1.Size())
	require.Equal(t, 0, pkt1.StreamIndex())

	pkt2 := AllocPacket()
	require.NotNil(t, pkt2)
	defer pkt2.Free()
	require.Nil(t, pkt2.Data())
	err = pkt2.AllocPayload(5)
	require.NoError(t, err)
	require.Len(t, pkt2.Data(), 5)
	pkt2.SetDts(1)
	pkt2.SetDuration(2)
	pkt2.SetFlags(NewPacketFlags(3))
	pkt2.SetPos(4)
	pkt2.SetPts(5)
	pkt2.SetSize(6)
	pkt2.SetStreamIndex(7)
	require.Equal(t, int64(1), pkt2.Dts())
	require.Equal(t, int64(2), pkt2.Duration())
	require.Equal(t, NewPacketFlags(3), pkt2.Flags())
	require.Equal(t, int64(4), pkt2.Pos())
	require.Equal(t, int64(5), pkt2.Pts())
	require.Equal(t, 6, pkt2.Size())
	require.Equal(t, 7, pkt2.StreamIndex())

	pkt3 := pkt1.Clone()
	require.NotNil(t, pkt3)
	defer pkt3.Free()
	require.Equal(t, int64(512), pkt3.Duration())

	err = pkt3.Ref(pkt2)
	require.NoError(t, err)
	require.Equal(t, int64(2), pkt3.Duration())

	pkt3.MoveRef(pkt1)
	require.Equal(t, int64(512), pkt3.Duration())
	require.Equal(t, int64(0), pkt1.Duration())

	pkt3.Unref()
	require.Equal(t, int64(0), pkt3.Duration())

	pkt3.SetDts(10)
	pkt3.SetDuration(20)
	pkt3.SetPts(30)
	pkt3.RescaleTs(NewRational(1, 10), NewRational(1, 1))
	require.Equal(t, int64(1), pkt3.Dts())
	require.Equal(t, int64(2), pkt3.Duration())
	require.Equal(t, int64(3), pkt3.Pts())

	pkt4 := AllocPacket()
	require.NotNil(t, pkt4)
	defer pkt4.Free()
	b := []byte("test")
	require.NoError(t, pkt4.FromData(b))
	require.Equal(t, b, pkt4.Data())

	pkt5 := AllocPacket()
	require.NotNil(t, pkt5)
	defer pkt5.Free()
	b = []byte{1, 2, 3, 4}
	sd := pkt5.SideData()
	require.NoError(t, sd.Add(PacketSideDataTypeAudioServiceType, b))
	require.Equal(t, b, sd.Get(PacketSideDataTypeAudioServiceType))

	pkt6 := AllocPacket()
	require.NotNil(t, pkt6)
	defer pkt6.Free()
	b = []byte{}
	require.NoError(t, pkt6.FromData(b))
	require.Equal(t, b, pkt6.Data())
}
