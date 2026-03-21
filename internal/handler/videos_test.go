package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/paylock/internal/model"
)

func TestVideos_EmptyList(t *testing.T) {
	videos := mustNewVideoStore(t)
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
	videos := mustNewVideoStore(t)
	videos.Create("vid-001", "Title 1", 0, "")
	videos.Create("vid-002", "Title 2", 0, "")
	videos.SetReady("vid-001", "thumb1", "https://agg/v1/blobs/thumb1", "blob1", "https://agg/v1/blobs/blob1", "blob1", "https://agg/v1/blobs/blob1")

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

	ids := map[string]bool{}
	for _, v := range resp.Videos {
		ids[v.ID] = true
	}

	if !ids["vid-001"] || !ids["vid-002"] {
		t.Errorf("expected vid-001 and vid-002, got %v", ids)
	}
}

func TestVideos_ContentType(t *testing.T) {
	videos := mustNewVideoStore(t)
	h := NewVideos(videos)

	req := httptest.NewRequest(http.MethodGet, "/api/videos", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}
