package model

import (
	"sync"
	"time"
)

type Status string

const (
	StatusProcessing Status = "processing"
	StatusReady      Status = "ready"
	StatusFailed     Status = "failed"
)

type Video struct {
	ID           string  `json:"id"`
	Status       Status  `json:"status"`
	CreatedAt    string  `json:"created_at"`
	Duration     float64 `json:"duration,omitempty"`
	Error        string  `json:"error,omitempty"`
	WalrusBlobID string  `json:"walrus_blob_id,omitempty"`
}

type VideoStore struct {
	mu     sync.RWMutex
	videos map[string]*Video
}

func NewVideoStore() *VideoStore {
	return &VideoStore{
		videos: make(map[string]*Video),
	}
}

func (s *VideoStore) Create(id string) *Video {
	return s.CreateAt(id, time.Now().UTC())
}

func (s *VideoStore) CreateAt(id string, t time.Time) *Video {
	v := &Video{
		ID:        id,
		Status:    StatusProcessing,
		CreatedAt: t.Format(time.RFC3339),
	}
	s.mu.Lock()
	s.videos[id] = v
	s.mu.Unlock()
	return v
}

func (s *VideoStore) Restore(id string, status Status, t time.Time) *Video {
	v := &Video{
		ID:        id,
		Status:    status,
		CreatedAt: t.Format(time.RFC3339),
	}
	s.mu.Lock()
	s.videos[id] = v
	s.mu.Unlock()
	return v
}

func (s *VideoStore) Get(id string) (*Video, bool) {
	s.mu.RLock()
	v, ok := s.videos[id]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	copied := *v
	return &copied, true
}

func (s *VideoStore) SetReady(id string, duration float64) {
	s.mu.Lock()
	if v, ok := s.videos[id]; ok {
		v.Status = StatusReady
		v.Duration = duration
	}
	s.mu.Unlock()
}

func (s *VideoStore) SetFailed(id string, errMsg string) {
	s.mu.Lock()
	if v, ok := s.videos[id]; ok {
		v.Status = StatusFailed
		v.Error = errMsg
	}
	s.mu.Unlock()
}

func (s *VideoStore) SetWalrusBlobID(id string, blobID string) {
	s.mu.Lock()
	if v, ok := s.videos[id]; ok {
		v.WalrusBlobID = blobID
	}
	s.mu.Unlock()
}

func (s *VideoStore) List() []Video {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Video, 0, len(s.videos))
	for _, v := range s.videos {
		result = append(result, *v)
	}
	return result
}
