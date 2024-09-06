package hub

import (
	"context"
	"fmt"
	"sync"
	"time"

	"liveflow/log"
)

var (
	ErrNotFoundAudioClockRate = fmt.Errorf("audio clock rate not found")
	ErrNotFoundVideoClockRate = fmt.Errorf("video clock rate not found")
)

type MediaType int

const (
	Video MediaType = 1
	Audio           = 2
)

type CodecType string

const (
	CodecTypeVP8  CodecType = "vp8"
	CodecTypeH264 CodecType = "h264"
	CodecTypeOpus CodecType = "opus"
	CodecTypeAAC  CodecType = "aac"
)

type MediaSpec struct {
	MediaType MediaType
	ClockRate uint32
	CodecType CodecType
}

type Source interface {
	Name() string
	MediaSpecs() []MediaSpec
	StreamID() string
	Depth() int
}

func HasCodecType(specs []MediaSpec, codecType CodecType) bool {
	for _, spec := range specs {
		if spec.CodecType == codecType {
			return true
		}
	}
	return false
}

func AudioClockRate(specs []MediaSpec) (uint32, error) {
	for _, spec := range specs {
		if spec.MediaType == Audio {
			return spec.ClockRate, nil
		}
	}
	return 0, ErrNotFoundAudioClockRate
}

func VideoClockRate(specs []MediaSpec) (uint32, error) {
	for _, spec := range specs {
		if spec.MediaType == Video {
			return spec.ClockRate, nil
		}
	}
	return 0, ErrNotFoundVideoClockRate
}

// Hub struct: Manages data independently for each streamID and supports Pub/Sub mechanism.
type Hub struct {
	streams    map[string][]chan *FrameData // Stores channels for each streamID
	notifyChan chan Source                  // Channel for notifying when streamID is determined
	mu         sync.RWMutex                 // Mutex for concurrency
}

// NewHub : Hub constructor
func NewHub() *Hub {
	return &Hub{
		streams:    make(map[string][]chan *FrameData),
		notifyChan: make(chan Source, 1024), // Buffer size can be adjusted.
	}
}

func (h *Hub) Notify(ctx context.Context, streamID Source) {
	log.Info(ctx, "Notify", streamID.Name(), streamID.MediaSpecs())
	h.notifyChan <- streamID
}

// Publish : Publishes data to the given streamID.
func (h *Hub) Publish(streamID string, data *FrameData) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.streams[streamID]; !exists {
		h.streams[streamID] = make([]chan *FrameData, 0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	for _, ch := range h.streams[streamID] {
		select {
		case ch <- data:
		case <-ctx.Done():
			log.Warn(ctx, "publish timeout")
		}
	}
}

func (h *Hub) Unpublish(streamID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.streams[streamID]; !exists {
		return
	}

	for _, ch := range h.streams[streamID] {
		close(ch)
	}
	delete(h.streams, streamID)
}

// Subscribe : Subscribes to the given streamID.
func (h *Hub) Subscribe(streamID string) <-chan *FrameData {
	h.mu.RLock()
	defer h.mu.RUnlock()

	ch := make(chan *FrameData)
	h.streams[streamID] = append(h.streams[streamID], ch)
	return ch
}

// SubscribeToStreamID : Returns a channel that subscribes to notifications when a stream ID is determined.
func (h *Hub) SubscribeToStreamID() <-chan Source {
	return h.notifyChan
}

// RemoveStream : Function to remove unused streams (releases resources)
func (h *Hub) RemoveStream(streamID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if chs, exists := h.streams[streamID]; exists {
		for _, ch := range chs {
			close(ch)
		}
		delete(h.streams, streamID)
	}
}
