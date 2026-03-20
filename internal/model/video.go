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
	ID             string `json:"id"`
	Title          string `json:"title"`
	Status         Status `json:"status"`
	CreatedAt      string `json:"created_at"`
	Price          uint64 `json:"price"`
	Creator        string `json:"creator,omitempty"`
	PreviewBlobID  string `json:"preview_blob_id,omitempty"`
	PreviewBlobURL string `json:"preview_blob_url,omitempty"`
	FullBlobID     string `json:"full_blob_id,omitempty"`
	FullBlobURL    string `json:"full_blob_url,omitempty"`
	SuiObjectID    string `json:"sui_object_id,omitempty"`
	Error          string `json:"error,omitempty"`
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

func (s *VideoStore) Create(id, title string, price uint64, creator string) *Video {
	v := &Video{
		ID:        id,
		Title:     title,
		Status:    StatusProcessing,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Price:     price,
		Creator:   creator,
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

func (s *VideoStore) SetReady(id, previewBlobID, previewBlobURL, fullBlobID, fullBlobURL string) {
	s.mu.Lock()
	if v, ok := s.videos[id]; ok {
		v.Status = StatusReady
		v.PreviewBlobID = previewBlobID
		v.PreviewBlobURL = previewBlobURL
		v.FullBlobID = fullBlobID
		v.FullBlobURL = fullBlobURL
	}
	s.mu.Unlock()
}

func (s *VideoStore) SetSuiObjectID(id, suiObjectID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.videos[id]
	if !ok {
		return false
	}
	v.SuiObjectID = suiObjectID
	return true
}

func (s *VideoStore) SetFailed(id string, errMsg string) {
	s.mu.Lock()
	if v, ok := s.videos[id]; ok {
		v.Status = StatusFailed
		v.Error = errMsg
	}
	s.mu.Unlock()
}

func (s *VideoStore) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.videos[id]; !ok {
		return false
	}
	delete(s.videos, id)
	return true
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
