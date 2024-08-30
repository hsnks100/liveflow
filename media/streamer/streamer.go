package streamer

import (
	"context"

	"liveflow/media/hub"
)

type Streamer interface {
	Start(ctx context.Context, hub hub.Hub) error
}
