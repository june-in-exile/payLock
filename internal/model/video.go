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
	mu          sync.RWMutex
	videos      map[string]*Video
	byObjectID  map[string]*Video // secondary index: sui_object_id → *Video
	filePath    string
	subscribers map[string][]chan Video
}

func NewVideoStore(dataDir string) (*VideoStore, error) {
	s := &VideoStore{
		videos:      make(map[string]*Video),
		byObjectID:  make(map[string]*Video),
		subscribers: make(map[string][]chan Video),
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

// Subscribe returns a channel that receives the video when its status changes
// to ready or failed. The returned cancel function removes the subscription.
func (s *VideoStore) Subscribe(id string) (<-chan Video, func()) {
	ch := make(chan Video, 1)
	s.mu.Lock()
	s.subscribers[id] = append(s.subscribers[id], ch)
	s.mu.Unlock()

	cancelled := false
	cancel := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if cancelled {
			return
		}
		cancelled = true
		subs := s.subscribers[id]
		for i, sub := range subs {
			if sub == ch {
				s.subscribers[id] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		if len(s.subscribers[id]) == 0 {
			delete(s.subscribers, id)
		}
	}
	return ch, cancel
}

// SubscribeIfProcessing atomically checks video status and subscribes if still processing.
// This eliminates the race condition between checking status and subscribing.
// Returns (nil, video, true) if video is already terminal (ready/failed).
// Returns (ch, video, true) if subscribed successfully (still processing).
// Returns (nil, nil, false) if video not found.
func (s *VideoStore) SubscribeIfProcessing(id string) (<-chan Video, *Video, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.videos[id]
	if !ok {
		return nil, nil, false
	}
	copied := *v
	if v.Status == StatusReady || v.Status == StatusFailed {
		return nil, &copied, true
	}
	ch := make(chan Video, 1)
	s.subscribers[id] = append(s.subscribers[id], ch)
	return ch, &copied, true
}

// ResolveAndSubscribeIfProcessing resolves by paylock_id or sui_object_id,
// then atomically subscribes if still processing.
// Returns (ch, video, canonical, true) or (nil, nil, false, false) if not found.
func (s *VideoStore) ResolveAndSubscribeIfProcessing(id string) (<-chan Video, *Video, bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	v, ok := s.videos[id]
	canonical := false
	if !ok {
		v, ok = s.byObjectID[id]
		canonical = true
	}
	if !ok {
		return nil, nil, false, false
	}

	copied := *v
	if v.Status == StatusReady || v.Status == StatusFailed {
		return nil, &copied, canonical, true
	}
	ch := make(chan Video, 1)
	s.subscribers[v.ID] = append(s.subscribers[v.ID], ch)
	return ch, &copied, canonical, true
}

// notify sends the video to all subscribers for the given ID. Must be called with mu held.
func (s *VideoStore) notify(id string) {
	v, ok := s.videos[id]
	if !ok {
		return
	}
	copied := *v
	for _, ch := range s.subscribers[id] {
		select {
		case ch <- copied:
		default:
		}
	}
	delete(s.subscribers, id)
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
	defer s.mu.Unlock()
	s.videos[id] = v
	s.persist()
	return v
}

func (s *VideoStore) Get(id string) (*Video, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.videos[id]
	if !ok {
		return nil, false
	}
	copied := *v
	return &copied, true
}

// GetBySuiObjectID looks up a video by its on-chain Sui object ID.
func (s *VideoStore) GetBySuiObjectID(objectID string) (*Video, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.byObjectID[objectID]
	if !ok {
		return nil, false
	}
	copied := *v
	return &copied, true
}

// Resolve looks up a video by paylock_id first, then falls back to sui_object_id.
// Returns the video and whether the lookup was by sui_object_id (canonical).
func (s *VideoStore) Resolve(id string) (video *Video, canonical bool, found bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.videos[id]; ok {
		copied := *v
		return &copied, false, true
	}
	if v, ok := s.byObjectID[id]; ok {
		copied := *v
		return &copied, true, true
	}
	return nil, false, false
}

func (s *VideoStore) SetReady(id, thumbnailBlobID, thumbnailBlobURL, previewBlobID, previewBlobURL, fullBlobID, fullBlobURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.videos[id]; ok {
		v.Status = StatusReady
		v.ThumbnailBlobID = thumbnailBlobID
		v.ThumbnailBlobURL = thumbnailBlobURL
		v.PreviewBlobID = previewBlobID
		v.PreviewBlobURL = previewBlobURL
		v.FullBlobID = fullBlobID
		v.FullBlobURL = fullBlobURL
		s.persist()
		s.notify(id)
	}
}

// SetPreviewUploaded stores the preview and thumbnail blob IDs for a paid video
// while keeping the status as "processing". The video transitions to "ready"
// only when SetSuiObjectID is called after on-chain publish.
func (s *VideoStore) SetPreviewUploaded(id, thumbnailBlobID, thumbnailBlobURL, previewBlobID, previewBlobURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.videos[id]; ok {
		v.ThumbnailBlobID = thumbnailBlobID
		v.ThumbnailBlobURL = thumbnailBlobURL
		v.PreviewBlobID = previewBlobID
		v.PreviewBlobURL = previewBlobURL
		s.persist()
		s.notify(id)
	}
}

func (s *VideoStore) SetSuiObjectID(id, suiObjectID, fullBlobID, fullBlobURL string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.videos[id]
	if !ok {
		return false
	}
	v.SuiObjectID = suiObjectID
	s.byObjectID[suiObjectID] = v
	if fullBlobID != "" {
		v.FullBlobID = fullBlobID
		v.FullBlobURL = fullBlobURL
	}
	if v.Status == StatusProcessing {
		v.Status = StatusReady
		s.notify(id)
	}
	s.persist()
	return true
}

func (s *VideoStore) SetFailed(id string, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.videos[id]; ok {
		v.Status = StatusFailed
		v.Error = errMsg
		s.persist()
		s.notify(id)
	}
}

func (s *VideoStore) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.videos[id]
	if !ok {
		return false
	}
	if v.SuiObjectID != "" {
		delete(s.byObjectID, v.SuiObjectID)
	}
	delete(s.videos, id)
	s.persist()
	return true
}

// UpsertFromChain creates or updates a video entry from on-chain data.
// If a video with the given sui_object_id already exists, it updates blob IDs.
// Otherwise, it creates a new entry using the sui_object_id as the video ID.
// Returns true if a new entry was created.
func (s *VideoStore) UpsertFromChain(suiObjectID string, price uint64, creator, previewBlobID, previewBlobURL, fullBlobID, fullBlobURL string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if we already have this object indexed.
	if v, ok := s.byObjectID[suiObjectID]; ok {
		// Update blob IDs if they were missing locally.
		changed := false
		if v.PreviewBlobID == "" && previewBlobID != "" {
			v.PreviewBlobID = previewBlobID
			v.PreviewBlobURL = previewBlobURL
			changed = true
		}
		if v.FullBlobID == "" && fullBlobID != "" {
			v.FullBlobID = fullBlobID
			v.FullBlobURL = fullBlobURL
			changed = true
		}
		if changed {
			s.persist()
		}
		return false
	}

	// Create a new entry using sui_object_id as both the ID and the object reference.
	v := &Video{
		ID:              suiObjectID,
		Title:           "",
		Status:          StatusReady,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
		Price:           price,
		Creator:         creator,
		Encrypted:       price > 0,
		SuiObjectID:     suiObjectID,
		PreviewBlobID:   previewBlobID,
		PreviewBlobURL:  previewBlobURL,
		FullBlobID:      fullBlobID,
		FullBlobURL:     fullBlobURL,
	}
	s.videos[suiObjectID] = v
	s.byObjectID[suiObjectID] = v
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
	staleCount := 0
	for _, v := range videos {
		if v.Status == StatusProcessing {
			v.Status = StatusFailed
			v.Error = "interrupted by server restart"
			staleCount++
		}
		s.videos[v.ID] = v
		if v.SuiObjectID != "" {
			s.byObjectID[v.SuiObjectID] = v
		}
	}
	if staleCount > 0 {
		slog.Warn("marked stale processing videos as failed", "count", staleCount)
		s.persist()
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
