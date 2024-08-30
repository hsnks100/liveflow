package hub

import (
	"context"
	"fmt"
	"sync"
	"time"

	"liveflow/log"
)

type MediaType int

const (
	Video MediaType = 1
	Audio           = 2
)

type MediaSpec struct {
	MediaType MediaType
	CodecType string
}

type Source interface {
	Name() string
	MediaSpecs() []MediaSpec
	StreamID() string
}

// Hub 구조체: streamID별로 독립적으로 데이터를 관리하고, Pub/Sub 메커니즘을 지원합니다.
type Hub struct {
	streams    map[string][]chan *FrameData // 각 streamID에 대한 채널을 저장
	notifyChan chan Source                  // streamID가 결정되었을 때 노티하는 채널
	mu         sync.RWMutex                 // 동시성을 위한 Mutex
}

// NewHub : Hub 생성자
func NewHub() *Hub {
	return &Hub{
		streams:    make(map[string][]chan *FrameData),
		notifyChan: make(chan Source, 1024), // 버퍼 크기를 조절할 수 있습니다.
	}
}

func (h *Hub) Notify(ctx context.Context, streamID Source) {
	log.Info(ctx, "Notify", streamID.Name(), streamID.MediaSpecs())
	h.notifyChan <- streamID
}

// Publish : 주어진 streamID에 데이터를 Publish합니다.
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
			fmt.Println("timeout~")
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

// Subscribe : 주어진 streamID에 대해 구독합니다.
func (h *Hub) Subscribe(streamID string) <-chan *FrameData {
	h.mu.RLock()
	defer h.mu.RUnlock()

	ch := make(chan *FrameData)
	h.streams[streamID] = append(h.streams[streamID], ch)
	return ch
}

// SubscribeToStreamID : 스트림 ID가 결정되었을 때 이를 구독하는 채널을 반환합니다.
func (h *Hub) SubscribeToStreamID() <-chan Source {
	return h.notifyChan
}

// RemoveStream : 사용하지 않는 스트림을 제거하는 함수 (리소스 해제)
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
