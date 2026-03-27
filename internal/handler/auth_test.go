package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/anthropics/paylock/internal/suiauth"
)

type mockVerifier struct {
	address string
	err     error
}

func (m *mockVerifier) Verify(_, _ string) (string, error) {
	return m.address, m.err
}

func nowTS() string {
	return strconv.FormatInt(time.Now().Unix(), 10)
}

func setAuthHeaders(req *http.Request, addr, sig, ts string) {
	req.Header.Set("X-Wallet-Address", addr)
	req.Header.Set("X-Wallet-Sig", sig)
	req.Header.Set("X-Wallet-Timestamp", ts)
}

// --- Delete tests ---

func TestDelete_AllowsNoAuthBeforeOnChain(t *testing.T) {
	videos := mustNewVideoStore(t)
	videos.Create("vid-001", "Test", 100, "0xAlice")
	h := NewDelete(videos, &mockVerifier{}, suiauth.FixedClock(time.Now().Unix()))

	req := httptest.NewRequest(http.MethodDelete, "/api/videos/vid-001", nil)
	req.SetPathValue("id", "vid-001")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for pre-chain video without auth, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDelete_RequiresCreatorAuth(t *testing.T) {
	videos := mustNewVideoStore(t)
	videos.Create("vid-001", "Test", 100, "0xAlice")
	videos.SetSuiObjectID("vid-001", "0xOBJ1", "blob1", "https://agg/blob1")
	v := &mockVerifier{address: "0xBob", err: nil}
	h := NewDelete(videos, v, suiauth.FixedClock(time.Now().Unix()))

	req := httptest.NewRequest(http.MethodDelete, "/api/videos/vid-001", nil)
	req.SetPathValue("id", "vid-001")
	setAuthHeaders(req, "0xBob", "fakesig", nowTS())
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for wrong creator, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDelete_AllowsCorrectCreator(t *testing.T) {
	videos := mustNewVideoStore(t)
	videos.Create("vid-001", "Test", 100, "0xAlice")
	videos.SetSuiObjectID("vid-001", "0xOBJ1", "blob1", "https://agg/blob1")
	v := &mockVerifier{address: "0xAlice", err: nil}
	h := NewDelete(videos, v, suiauth.FixedClock(time.Now().Unix()))

	req := httptest.NewRequest(http.MethodDelete, "/api/videos/vid-001", nil)
	req.SetPathValue("id", "vid-001")
	setAuthHeaders(req, "0xAlice", "fakesig", nowTS())
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for correct creator, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDelete_AllowsCaseInsensitiveCreator(t *testing.T) {
	videos := mustNewVideoStore(t)
	videos.Create("vid-001", "Test", 100, "0xAbCdEf")
	videos.SetSuiObjectID("vid-001", "0xOBJ1", "blob1", "https://agg/blob1")
	v := &mockVerifier{address: "0xabcdef", err: nil}
	h := NewDelete(videos, v, suiauth.FixedClock(time.Now().Unix()))

	req := httptest.NewRequest(http.MethodDelete, "/api/videos/vid-001", nil)
	req.SetPathValue("id", "vid-001")
	setAuthHeaders(req, "0xabcdef", "fakesig", nowTS())
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for case-insensitive creator match, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDelete_AllowsNoCreatorVideo(t *testing.T) {
	videos := mustNewVideoStore(t)
	videos.Create("vid-001", "Test", 0, "")
	h := NewDelete(videos, nil, suiauth.FixedClock(time.Now().Unix()))

	req := httptest.NewRequest(http.MethodDelete, "/api/videos/vid-001", nil)
	req.SetPathValue("id", "vid-001")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for video without creator, got %d", rec.Code)
	}
}

func TestDelete_MissingAuthHeaders(t *testing.T) {
	videos := mustNewVideoStore(t)
	videos.Create("vid-001", "Test", 100, "0xAlice")
	videos.SetSuiObjectID("vid-001", "0xOBJ1", "blob1", "https://agg/blob1")
	h := NewDelete(videos, &mockVerifier{}, suiauth.FixedClock(time.Now().Unix()))

	req := httptest.NewRequest(http.MethodDelete, "/api/videos/vid-001", nil)
	req.SetPathValue("id", "vid-001")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth headers, got %d", rec.Code)
	}
}

func TestDelete_InvalidSignature(t *testing.T) {
	videos := mustNewVideoStore(t)
	videos.Create("vid-001", "Test", 100, "0xAlice")
	videos.SetSuiObjectID("vid-001", "0xOBJ1", "blob1", "https://agg/blob1")
	v := &mockVerifier{address: "", err: errors.New("bad sig")}
	h := NewDelete(videos, v, suiauth.FixedClock(time.Now().Unix()))

	req := httptest.NewRequest(http.MethodDelete, "/api/videos/vid-001", nil)
	req.SetPathValue("id", "vid-001")
	setAuthHeaders(req, "0xAlice", "badsig", nowTS())
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid signature, got %d", rec.Code)
	}
}

func TestDelete_ExpiredTimestamp(t *testing.T) {
	videos := mustNewVideoStore(t)
	videos.Create("vid-001", "Test", 100, "0xAlice")
	videos.SetSuiObjectID("vid-001", "0xOBJ1", "blob1", "https://agg/blob1")
	v := &mockVerifier{address: "0xAlice", err: nil}
	// Clock is at T=1000, but request timestamp will be T=100 (900s ago)
	h := NewDelete(videos, v, suiauth.FixedClock(1000))

	req := httptest.NewRequest(http.MethodDelete, "/api/videos/vid-001", nil)
	req.SetPathValue("id", "vid-001")
	setAuthHeaders(req, "0xAlice", "fakesig", "100")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for expired timestamp, got %d", rec.Code)
	}
}

// --- Pagination tests ---

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
