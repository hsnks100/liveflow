package hub

import "fmt"

type SliceType int

func (s SliceType) String() string {
	switch s {
	case SliceI:
		return "I"
	case SliceP:
		return "P"
	case SliceB:
		return "B"
	case SliceSPS:
		return "SPS"
	case SlicePPS:
		return "PPS"
	default:
		return fmt.Sprintf("Unknown SliceType: %d", s)
	}
}

const (
	SliceI       SliceType = 0
	SliceP       SliceType = 1
	SliceB       SliceType = 2
	SliceSPS     SliceType = 3
	SlicePPS     SliceType = 4
	SliceUnknown SliceType = 5
)
