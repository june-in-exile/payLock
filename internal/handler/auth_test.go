package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDelete_RequiresCreatorAuth(t *testing.T) {
	videos := mustNewVideoStore(t)
	videos.Create("vid-001", "Test", 100, "0xAlice")
	h := NewDelete(videos)

	req := httptest.NewRequest(http.MethodDelete, "/api/videos/vid-001", nil)
	req.SetPathValue("id", "vid-001")
	req.Header.Set("X-Creator", "0xBob")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for wrong creator, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDelete_AllowsCorrectCreator(t *testing.T) {
	videos := mustNewVideoStore(t)
	videos.Create("vid-001", "Test", 100, "0xAlice")
	h := NewDelete(videos)

	req := httptest.NewRequest(http.MethodDelete, "/api/videos/vid-001", nil)
	req.SetPathValue("id", "vid-001")
	req.Header.Set("X-Creator", "0xAlice")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for correct creator, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDelete_AllowsNoCreatorVideo(t *testing.T) {
	videos := mustNewVideoStore(t)
	videos.Create("vid-001", "Test", 0, "")
	h := NewDelete(videos)

	req := httptest.NewRequest(http.MethodDelete, "/api/videos/vid-001", nil)
	req.SetPathValue("id", "vid-001")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for video without creator, got %d", rec.Code)
	}
}

func TestDelete_MissingCreatorHeader(t *testing.T) {
	videos := mustNewVideoStore(t)
	videos.Create("vid-001", "Test", 100, "0xAlice")
	h := NewDelete(videos)

	req := httptest.NewRequest(http.MethodDelete, "/api/videos/vid-001", nil)
	req.SetPathValue("id", "vid-001")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without X-Creator header, got %d", rec.Code)
	}
}

func TestSetSuiObject_RequiresCreatorAuth(t *testing.T) {
	videos := mustNewVideoStore(t)
	videos.Create("vid-001", "Test", 100, "0xAlice")
	videos.SetReady("vid-001", "", "", "prev1", "https://agg/prev1", "", "")

	store := &mockStorer{storeFunc: func(data []byte, epochs int) (string, error) {
		return "blob1", nil
	}}
	h := NewSetSuiObject(videos, store)

	body := `{"sui_object_id":"0xOBJ1","full_blob_id":"blob99"}`
	req := httptest.NewRequest(http.MethodPut, "/api/videos/vid-001/sui-object", strings.NewReader(body))
	req.SetPathValue("id", "vid-001")
	req.Header.Set("X-Creator", "0xBob")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for wrong creator, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSetSuiObject_AllowsCorrectCreator(t *testing.T) {
	videos := mustNewVideoStore(t)
	videos.Create("vid-001", "Test", 100, "0xAlice")
	videos.SetReady("vid-001", "", "", "prev1", "https://agg/prev1", "", "")

	store := &mockStorer{storeFunc: func(data []byte, epochs int) (string, error) {
		return "blob1", nil
	}}
	h := NewSetSuiObject(videos, store)

	body := `{"sui_object_id":"0xOBJ1","full_blob_id":"blob99"}`
	req := httptest.NewRequest(http.MethodPut, "/api/videos/vid-001/sui-object", strings.NewReader(body))
	req.SetPathValue("id", "vid-001")
	req.Header.Set("X-Creator", "0xAlice")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for correct creator, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestVideos_Pagination_DefaultValues(t *testing.T) {
	videos := mustNewVideoStore(t)
	for i := 0; i < 25; i++ {
		videos.Create(fmt.Sprintf("vid-%03d", i), fmt.Sprintf("Video %d", i), 0, "")
	}
	h := NewVideos(videos)

	req := httptest.NewRequest(http.MethodGet, "/api/videos", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	var resp struct {
		Videos  []interface{} `json:"videos"`
		Total   int           `json:"total"`
		Page    int           `json:"page"`
		PerPage int           `json:"per_page"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp.Total != 25 {
		t.Errorf("expected total=25, got %d", resp.Total)
	}
	if resp.Page != 1 {
		t.Errorf("expected page=1, got %d", resp.Page)
	}
	if resp.PerPage != 20 {
		t.Errorf("expected per_page=20, got %d", resp.PerPage)
	}
	if len(resp.Videos) != 20 {
		t.Errorf("expected 20 videos on page 1, got %d", len(resp.Videos))
	}
}

func TestVideos_Pagination_SecondPage(t *testing.T) {
	videos := mustNewVideoStore(t)
	for i := 0; i < 25; i++ {
		videos.Create(fmt.Sprintf("vid-%03d", i), fmt.Sprintf("Video %d", i), 0, "")
	}
	h := NewVideos(videos)

	req := httptest.NewRequest(http.MethodGet, "/api/videos?page=2", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	var resp struct {
		Videos  []interface{} `json:"videos"`
		Total   int           `json:"total"`
		Page    int           `json:"page"`
		PerPage int           `json:"per_page"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)

	if len(resp.Videos) != 5 {
		t.Errorf("expected 5 videos on page 2, got %d", len(resp.Videos))
	}
	if resp.Page != 2 {
		t.Errorf("expected page=2, got %d", resp.Page)
	}
}

func TestVideos_Pagination_CustomPerPage(t *testing.T) {
	videos := mustNewVideoStore(t)
	for i := 0; i < 10; i++ {
		videos.Create(fmt.Sprintf("vid-%03d", i), fmt.Sprintf("Video %d", i), 0, "")
	}
	h := NewVideos(videos)

	req := httptest.NewRequest(http.MethodGet, "/api/videos?per_page=3&page=2", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	var resp struct {
		Videos  []interface{} `json:"videos"`
		Total   int           `json:"total"`
		Page    int           `json:"page"`
		PerPage int           `json:"per_page"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)

	if len(resp.Videos) != 3 {
		t.Errorf("expected 3 videos, got %d", len(resp.Videos))
	}
	if resp.Total != 10 {
		t.Errorf("expected total=10, got %d", resp.Total)
	}
}

func TestVideos_Pagination_WithCreatorFilter(t *testing.T) {
	videos := mustNewVideoStore(t)
	for i := 0; i < 5; i++ {
		videos.Create(fmt.Sprintf("alice-%d", i), fmt.Sprintf("Alice %d", i), 0, "0xAlice")
	}
	for i := 0; i < 3; i++ {
		videos.Create(fmt.Sprintf("bob-%d", i), fmt.Sprintf("Bob %d", i), 0, "0xBob")
	}
	h := NewVideos(videos)

	req := httptest.NewRequest(http.MethodGet, "/api/videos?creator=0xAlice&per_page=2&page=1", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	var resp struct {
		Videos  []interface{} `json:"videos"`
		Total   int           `json:"total"`
		Page    int           `json:"page"`
		PerPage int           `json:"per_page"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp.Total != 5 {
		t.Errorf("expected total=5 (Alice's videos), got %d", resp.Total)
	}
	if len(resp.Videos) != 2 {
		t.Errorf("expected 2 videos on page 1, got %d", len(resp.Videos))
	}
}
