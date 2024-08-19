package hlshub

import (
	"errors"
	"sync"

	"github.com/bluenviron/gohlslib"
)

var (
	errNotFoundStream = errors.New("no HLS stream")
)

type HLSHub struct {
	mu sync.RWMutex
	// [workID][name(low|pass)]muxer
	hlsMuxers map[string]map[string]*gohlslib.Muxer
}

func NewHLSHub() *HLSHub {
	return &HLSHub{
		mu:        sync.RWMutex{},
		hlsMuxers: map[string]map[string]*gohlslib.Muxer{},
	}
}

func (s *HLSHub) StoreMuxer(workID string, name string, muxer *gohlslib.Muxer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hlsMuxers[workID] == nil {
		s.hlsMuxers[workID] = map[string]*gohlslib.Muxer{}
	}
	s.hlsMuxers[workID][name] = muxer
}

func (s *HLSHub) DeleteMuxer(workID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.hlsMuxers, workID)
}

func (s *HLSHub) Muxer(workID string, name string) (*gohlslib.Muxer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	muxers, prs := s.hlsMuxers[workID]
	if !prs {
		return nil, errNotFoundStream
	}
	for n, muxer := range muxers {
		if n == name {
			return muxer, nil
		}
	}
	return nil, errNotFoundStream
}

func (s *HLSHub) MuxersByWorkID(workID string) (map[string]*gohlslib.Muxer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	muxers, prs := s.hlsMuxers[workID]
	if !prs {
		return nil, errNotFoundStream
	}
	return muxers, nil
}
