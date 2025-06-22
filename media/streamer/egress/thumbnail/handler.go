package thumbnail

import (
	"context"
	"liveflow/media/hub"
)

// ThumbnailArgs contains arguments for initializing thumbnail service
type ThumbnailArgs struct {
	Hub             *hub.Hub
	OutputPath      string // Path to save thumbnails
	IntervalSeconds int    // Thumbnail generation interval in seconds
	Width           int    // Thumbnail width
	Height          int    // Thumbnail height
}

// Thumbnail represents thumbnail generation service
type Thumbnail struct {
	hub             *hub.Hub
	outputPath      string
	intervalSeconds int
	width           int
	height          int
}

// NewThumbnail creates a new thumbnail service instance
func NewThumbnail(args ThumbnailArgs) *Thumbnail {
	return &Thumbnail{
		hub:             args.Hub,
		outputPath:      args.OutputPath,
		intervalSeconds: args.IntervalSeconds,
		width:           args.Width,
		height:          args.Height,
	}
}

// Start starts the thumbnail service
func (t *Thumbnail) Start(ctx context.Context, source hub.Source) error {
	// TODO: implement thumbnail generation logic
	return nil
}

// Name returns the service name
func (t *Thumbnail) Name() string {
	return "thumbnail"
}

// Stop stops the thumbnail service
func (t *Thumbnail) Stop() error {
	// TODO: implement cleanup logic
	return nil
}
