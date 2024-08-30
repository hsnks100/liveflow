package whep

import (
	"context"

	"liveflow/media/hub"
)

type WHEPArgs struct {
	Hub *hub.Hub
}

// whip
type WHEP struct {
	hub *hub.Hub
}

func NewWHEP(args WHEPArgs) *WHEP {
	return &WHEP{
		hub: args.Hub,
	}
}

func (w *WHEP) Start(ctx context.Context, source hub.Source) error {
	sub := w.hub.Subscribe(source.StreamID())
	go func() {
		for data := range sub {
			_ = data
		}
	}()
	return nil
}
