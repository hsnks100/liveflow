package repeater

import (
	"context"
	"liveflow/log"
	"liveflow/media/hub"
	"liveflow/media/streamer/fields"

	"github.com/sirupsen/logrus"
)

// 1 input, multiple outputs
type Pipe[I any, O any] struct {
}
type Repeater struct {
	hub *hub.Hub
}

type RepeaterArgs struct {
	Hub *hub.Hub
}

func NewRepeater(args RepeaterArgs) *Repeater {
	return &Repeater{
		hub: args.Hub,
	}
}

func (r *Repeater) Start(ctx context.Context, source hub.Source) error {
	ctx = log.WithFields(ctx, logrus.Fields{
		fields.StreamID:   source.StreamID(),
		fields.SourceName: source.Name(),
	})
	log.Info(ctx, "start Repeater")
	sub := r.hub.Subscribe(source.StreamID())

	//go func() {
	//	for data := range sub {
	//		r.hub.Publish(source.StreamID(), data)
	//	}
	//}()
	return nil
}
