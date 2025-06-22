package ingress

import (
	"liveflow/media/hub"

	"github.com/deepch/vdk/codec/h264parser"
)

func SliceTypes(payload []byte) []hub.SliceType {
	nalus, _ := h264parser.SplitNALUs(payload)
	slices := make([]hub.SliceType, 0)
	for _, nalu := range nalus {
		if len(nalu) < 1 {
			continue
		}
		nalUnitType := nalu[0] & 0x1f
		switch nalUnitType {
		case h264parser.NALU_SPS:
			slices = append(slices, hub.SliceSPS)
		case h264parser.NALU_PPS:
			slices = append(slices, hub.SlicePPS)
		default:
			sliceType, _ := h264parser.ParseSliceHeaderFromNALU(nalu)
			switch sliceType {
			case h264parser.SLICE_I:
				slices = append(slices, hub.SliceI)
			case h264parser.SLICE_P:
				slices = append(slices, hub.SliceP)
			case h264parser.SLICE_B:
				slices = append(slices, hub.SliceB)
			}
		}
	}
	return slices
}
