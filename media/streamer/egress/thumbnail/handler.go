package thumbnail

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"sync"
	"time"

	"liveflow/log"
	"liveflow/media/hub"
	"liveflow/media/streamer/fields"
	"liveflow/media/streamer/processes"

	"github.com/asticode/go-astiav"
	"github.com/sirupsen/logrus"
)

var (
	ErrUnsupportedCodec = errors.New("unsupported codec")
)

// ThumbnailData represents thumbnail data in memory
type ThumbnailData struct {
	Data      []byte
	Timestamp time.Time
}

// ThumbnailStore manages thumbnails in memory
type ThumbnailStore struct {
	mu         sync.RWMutex
	thumbnails map[string]*ThumbnailData
}

// NewThumbnailStore creates a new thumbnail store
func NewThumbnailStore() *ThumbnailStore {
	return &ThumbnailStore{
		thumbnails: make(map[string]*ThumbnailData),
	}
}

// Set stores thumbnail data for a stream
func (ts *ThumbnailStore) Set(streamID string, data []byte) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.thumbnails[streamID] = &ThumbnailData{
		Data:      data,
		Timestamp: time.Now(),
	}
}

// Get retrieves thumbnail data for a stream
func (ts *ThumbnailStore) Get(streamID string) (*ThumbnailData, bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	data, exists := ts.thumbnails[streamID]
	return data, exists
}

// Delete removes thumbnail data for a stream
func (ts *ThumbnailStore) Delete(streamID string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	delete(ts.thumbnails, streamID)
}

// ThumbnailArgs contains arguments for initializing thumbnail service
type ThumbnailArgs struct {
	Hub             *hub.Hub
	Store           *ThumbnailStore
	OutputPath      string // Not used anymore, kept for compatibility
	IntervalSeconds int    // Thumbnail generation interval in seconds
	Width           int    // Thumbnail width
	Height          int    // Thumbnail height
}

// Thumbnail represents thumbnail generation service
type Thumbnail struct {
	hub               *hub.Hub
	intervalSeconds   int
	width             int
	height            int
	decoder           *processes.VideoDecodingProcess
	lastThumbnailTime int64
	store             *ThumbnailStore
}

// NewThumbnail creates a new thumbnail service instance
func NewThumbnail(args ThumbnailArgs) *Thumbnail {
	return &Thumbnail{
		hub:             args.Hub,
		intervalSeconds: args.IntervalSeconds,
		width:           args.Width,
		height:          args.Height,
		store:           args.Store,
	}
}

// Start starts the thumbnail service
func (t *Thumbnail) Start(ctx context.Context, source hub.Source) error {
	if !hub.HasCodecType(source.MediaSpecs(), hub.CodecTypeH264) {
		return ErrUnsupportedCodec
	}

	ctx = log.WithFields(ctx, logrus.Fields{
		fields.StreamID:   source.StreamID(),
		fields.SourceName: source.Name(),
	})
	log.Info(ctx, "start thumbnail")

	// Initialize video decoder
	t.decoder = processes.NewVideoDecodingProcess(astiav.CodecIDH264)
	if err := t.decoder.Init(); err != nil {
		return err
	}

	sub := t.hub.Subscribe(source.StreamID())
	go func() {
		intervalMS := int64(t.intervalSeconds * 1000)

		for data := range sub {
			if data.H264Video != nil {
				// Check if enough time has passed for next thumbnail
				if data.H264Video.RawDTS()-t.lastThumbnailTime >= intervalMS {
					// Check if this is a keyframe for better thumbnail quality
					isKeyFrame := false
					for _, sliceType := range data.H264Video.SliceTypes {
						if sliceType == hub.SliceI {
							isKeyFrame = true
							break
						}
					}

					if isKeyFrame {
						t.onVideo(ctx, data.H264Video, source.StreamID())
						t.lastThumbnailTime = data.H264Video.RawDTS()
					}
				}
			}
		}

		// Clean up thumbnail when stream ends
		t.store.Delete(source.StreamID())
		log.Infof(ctx, "thumbnail cleaned up for stream %s", source.StreamID())
	}()

	return nil
}

// encodeImageToJPEG encodes an image to JPEG bytes
func (t *Thumbnail) encodeImageToJPEG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer

	// JPEG encode options
	options := &jpeg.Options{
		Quality: 85, // Good quality for thumbnails
	}

	if err := jpeg.Encode(&buf, img, options); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// onVideo processes H264 video data and generates thumbnail
func (t *Thumbnail) onVideo(ctx context.Context, h264Video *hub.H264Video, streamID string) {
	// Decode H264 to AVFrame
	frames, err := t.decoder.Process(*h264Video)
	if err != nil {
		log.Error(ctx, err, "failed to decode video for thumbnail")
		return
	}

	// Process each decoded frame
	for _, frame := range frames {
		if frame != nil {
			t.generateThumbnail(ctx, frame, streamID)
			// Only generate one thumbnail per interval
			break
		}
	}
}

// generateThumbnail creates thumbnail from AVFrame
func (t *Thumbnail) generateThumbnail(ctx context.Context, frame *astiav.Frame, streamID string) {
	// Get frame data
	frameData := frame.Data()

	// Guess the image format from pixel format
	img, err := frameData.GuessImageFormat()
	if err != nil {
		log.Error(ctx, err, "failed to guess image format")
		return
	}

	// Convert AVFrame to Go image
	err = frameData.ToImage(img)
	if err != nil {
		log.Error(ctx, err, "failed to convert frame to image")
		return
	}

	// Encode image to JPEG bytes
	jpegData, err := t.encodeImageToJPEG(img)
	if err != nil {
		log.Error(ctx, err, "failed to encode image to JPEG")
		return
	}

	// Store in memory
	t.store.Set(streamID, jpegData)

	log.Infof(ctx, "thumbnail updated in memory for stream %s (size: %d bytes, image: %dx%d)",
		streamID, len(jpegData), img.Bounds().Dx(), img.Bounds().Dy())
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
