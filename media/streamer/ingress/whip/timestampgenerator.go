package whip

import (
	"golang.org/x/exp/constraints"
)

type TimestampGenerator[T constraints.Unsigned | constraints.Signed] struct {
	firstTimestamp T
	received       bool
}

func (h *TimestampGenerator[T]) Initialized() bool {
	return h.received
}

func (h *TimestampGenerator[T]) GetTimestamp(timestamp T) T {
	if !h.received {
		h.firstTimestamp = timestamp
		h.received = true
	}
	return timestamp - h.firstTimestamp
}

func (h *TimestampGenerator[T]) Reset() {
	h.received = false
}
