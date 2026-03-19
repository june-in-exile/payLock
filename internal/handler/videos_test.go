package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anthropics/orca/internal/model"
)

func TestVideos_EmptyList(t *testing.T) {
	videos := model.NewVideoStore()
	h := NewVideos(videos)

	req := httptest.NewRequest(http.MethodGet, "/api/videos", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Videos []model.Video `json:"videos"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Videos) != 0 {
		t.Errorf("expected 0 videos, got %d", len(resp.Videos))
	}
}

func TestVideos_ReturnsList(t *testing.T) {
	videos := model.NewVideoStore()
	videos.Create("vid-001", "Title 1")
	videos.Create("vid-002", "Title 2")
	videos.SetReady("vid-001", 120.5)

	h := NewVideos(videos)

	req := httptest.NewRequest(http.MethodGet, "/api/videos", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Videos []model.Video `json:"videos"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Videos) != 2 {
		t.Fatalf("expected 2 videos, got %d", len(resp.Videos))
	}

	// Verify we have both videos
	ids := map[string]bool{}
	for _, v := range resp.Videos {
		ids[v.ID] = true
	}

	if !ids["vid-001"] || !ids["vid-002"] {
		t.Errorf("expected vid-001 and vid-002, got %v", ids)
	}
}

func TestVideos_SortedNewestFirst(t *testing.T) {
	videos := model.NewVideoStore()
	older := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	videos.CreateAt("older", "Older", older)
	videos.CreateAt("newer", "Newer", newer)

	h := NewVideos(videos)

	req := httptest.NewRequest(http.MethodGet, "/api/videos", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	var resp struct {
		Videos []model.Video `json:"videos"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Videos) != 2 {
		t.Fatalf("expected 2 videos, got %d", len(resp.Videos))
	}

	if resp.Videos[0].ID != "newer" {
		t.Errorf("expected newest video first, got %s", resp.Videos[0].ID)
	}
	if resp.Videos[1].ID != "older" {
		t.Errorf("expected oldest video last, got %s", resp.Videos[1].ID)
	}
}

func TestVideos_ContentType(t *testing.T) {
	videos := model.NewVideoStore()
	h := NewVideos(videos)

	req := httptest.NewRequest(http.MethodGet, "/api/videos", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}
