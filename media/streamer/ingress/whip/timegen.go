package whip

import (
	"golang.org/x/exp/constraints"
)

type TimestampGenerator[T constraints.Unsigned | constraints.Signed] struct {
	initialTimestamp T
	initialized      bool
}

func (g *TimestampGenerator[T]) IsInitialized() bool {
	return g.initialized
}

func (g *TimestampGenerator[T]) Generate(timestamp T) T {
	if !g.initialized {
		g.initialTimestamp = timestamp
		g.initialized = true
	}
	return timestamp - g.initialTimestamp
}

func (g *TimestampGenerator[T]) Reset() {
	g.initialized = false
}
