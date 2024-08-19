package streamer

import (
	"context"

	"mrw-clone/media/hub"
)

type Streamer interface {
	Start(ctx context.Context, hub hub.Hub) error
}
