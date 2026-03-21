package model

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
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
	ThumbnailBlobID  string `json:"thumbnail_blob_id,omitempty"`
	ThumbnailBlobURL string `json:"thumbnail_blob_url,omitempty"`
	PreviewBlobID    string `json:"preview_blob_id,omitempty"`
	PreviewBlobURL   string `json:"preview_blob_url,omitempty"`
	FullBlobID       string `json:"full_blob_id,omitempty"`
	FullBlobURL      string `json:"full_blob_url,omitempty"`
	Encrypted      bool   `json:"encrypted"`
	SuiObjectID    string `json:"sui_object_id,omitempty"`
	Error          string `json:"error,omitempty"`
}

type VideoStore struct {
	mu       sync.RWMutex
	videos   map[string]*Video
	filePath string
}

func NewVideoStore(dataDir string) (*VideoStore, error) {
	s := &VideoStore{
		videos: make(map[string]*Video),
	}

	if dataDir != "" {
		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			return nil, err
		}
		s.filePath = filepath.Join(dataDir, "videos.json")
		if err := s.load(); err != nil {
			return nil, err
		}
	}

	return s, nil
}

func (s *VideoStore) Create(id, title string, price uint64, creator string) *Video {
	v := &Video{
		ID:        id,
		Title:     title,
		Status:    StatusProcessing,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Price:     price,
		Creator:   creator,
		Encrypted: price > 0,
	}
	s.mu.Lock()
	s.videos[id] = v
	s.persist()
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

func (s *VideoStore) SetReady(id, thumbnailBlobID, thumbnailBlobURL, previewBlobID, previewBlobURL, fullBlobID, fullBlobURL string) {
	s.mu.Lock()
	if v, ok := s.videos[id]; ok {
		v.Status = StatusReady
		v.ThumbnailBlobID = thumbnailBlobID
		v.ThumbnailBlobURL = thumbnailBlobURL
		v.PreviewBlobID = previewBlobID
		v.PreviewBlobURL = previewBlobURL
		v.FullBlobID = fullBlobID
		v.FullBlobURL = fullBlobURL
		s.persist()
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
	s.persist()
	return true
}

func (s *VideoStore) SetFullBlob(id, fullBlobID, fullBlobURL string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.videos[id]
	if !ok {
		return false
	}
	v.FullBlobID = fullBlobID
	v.FullBlobURL = fullBlobURL
	s.persist()
	return true
}

func (s *VideoStore) SetFailed(id string, errMsg string) {
	s.mu.Lock()
	if v, ok := s.videos[id]; ok {
		v.Status = StatusFailed
		v.Error = errMsg
		s.persist()
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
	s.persist()
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

func (s *VideoStore) load() error {
	data, err := os.ReadFile(s.filePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var videos []*Video
	if err := json.Unmarshal(data, &videos); err != nil {
		return err
	}
	for _, v := range videos {
		s.videos[v.ID] = v
	}
	slog.Info("loaded videos from disk", "count", len(videos), "path", s.filePath)
	return nil
}

// persist writes the current video state to disk. Must be called with mu held.
func (s *VideoStore) persist() {
	if s.filePath == "" {
		return
	}
	videos := make([]*Video, 0, len(s.videos))
	for _, v := range s.videos {
		videos = append(videos, v)
	}
	data, err := json.MarshalIndent(videos, "", "  ")
	if err != nil {
		slog.Error("failed to marshal videos", "error", err)
		return
	}

	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		slog.Error("failed to write videos file", "error", err)
		return
	}
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		slog.Error("failed to rename videos file", "error", err)
	}
}
