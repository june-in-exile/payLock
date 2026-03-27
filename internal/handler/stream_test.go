package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStreamPreview_ByPayLockID(t *testing.T) {
	store := mustNewVideoStore(t)
	store.Create("abc123", "Test Video", 0, "")
	store.SetReady("abc123", "tb", "https://agg/v1/blobs/tb", "pb", "https://agg/v1/blobs/pb", "fb", "https://agg/v1/blobs/fb")

	h := NewStreamPreview(store)
	req := httptest.NewRequest(http.MethodGet, "/stream/abc123/preview", nil)
	req.SetPathValue("id", "abc123")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307, got %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "https://agg/v1/blobs/pb" {
		t.Errorf("expected redirect to preview blob URL, got %s", got)
	}
}

func TestStreamPreview_BySuiObjectID(t *testing.T) {
	store := mustNewVideoStore(t)
	store.Create("abc123", "Test Video", 0, "")
	store.SetReady("abc123", "tb", "https://agg/v1/blobs/tb", "pb", "https://agg/v1/blobs/pb", "fb", "https://agg/v1/blobs/fb")
	store.SetSuiObjectID("abc123", "0xOBJ999", "", "")

	h := NewStreamPreview(store)
	req := httptest.NewRequest(http.MethodGet, "/stream/0xOBJ999/preview", nil)
	req.SetPathValue("id", "0xOBJ999")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307, got %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "https://agg/v1/blobs/pb" {
		t.Errorf("expected redirect to preview blob URL, got %s", got)
	}
}

func TestStreamPreview_PayLockID_RedirectsToCanonical(t *testing.T) {
	store := mustNewVideoStore(t)
	store.Create("abc123", "Test Video", 0, "")
	store.SetReady("abc123", "tb", "https://agg/v1/blobs/tb", "pb", "https://agg/v1/blobs/pb", "fb", "https://agg/v1/blobs/fb")
	store.SetSuiObjectID("abc123", "0xOBJ999", "", "")

	h := NewStreamPreview(store)
	req := httptest.NewRequest(http.MethodGet, "/stream/abc123/preview", nil)
	req.SetPathValue("id", "abc123")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307, got %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/stream/0xOBJ999/preview" {
		t.Errorf("expected canonical redirect to /stream/0xOBJ999/preview, got %s", got)
	}
}

func TestStreamPreview_NotFound(t *testing.T) {
	store := mustNewVideoStore(t)
	h := NewStreamPreview(store)

	req := httptest.NewRequest(http.MethodGet, "/stream/unknown/preview", nil)
	req.SetPathValue("id", "unknown")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestStreamPreview_NotReady(t *testing.T) {
	store := mustNewVideoStore(t)
	store.Create("abc123", "Test Video", 0, "")

	h := NewStreamPreview(store)
	req := httptest.NewRequest(http.MethodGet, "/stream/abc123/preview", nil)
	req.SetPathValue("id", "abc123")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestStreamPreview_ByPayLockID_DeprecationHeaders(t *testing.T) {
	store := mustNewVideoStore(t)
	store.Create("abc123", "Test Video", 0, "")
	store.SetReady("abc123", "tb", "https://agg/v1/blobs/tb", "pb", "https://agg/v1/blobs/pb", "fb", "https://agg/v1/blobs/fb")
	// No sui_object_id set — should get deprecation headers instead of canonical redirect.

	h := NewStreamPreview(store)
	req := httptest.NewRequest(http.MethodGet, "/stream/abc123/preview", nil)
	req.SetPathValue("id", "abc123")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307, got %d", rec.Code)
	}
	if got := rec.Header().Get("Deprecation"); got != "true" {
		t.Errorf("expected Deprecation header, got %q", got)
	}
	if got := rec.Header().Get("Sunset"); got == "" {
		t.Error("expected Sunset header to be set")
	}
}

func TestStreamPreview_BySuiObjectID_NoDeprecationHeaders(t *testing.T) {
	store := mustNewVideoStore(t)
	store.Create("abc123", "Test Video", 0, "")
	store.SetReady("abc123", "tb", "https://agg/v1/blobs/tb", "pb", "https://agg/v1/blobs/pb", "fb", "https://agg/v1/blobs/fb")
	store.SetSuiObjectID("abc123", "0xOBJ999", "", "")

	h := NewStreamPreview(store)
	req := httptest.NewRequest(http.MethodGet, "/stream/0xOBJ999/preview", nil)
	req.SetPathValue("id", "0xOBJ999")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Deprecation"); got != "" {
		t.Errorf("expected no Deprecation header for canonical access, got %q", got)
	}
}

func TestStreamFull_BySuiObjectID(t *testing.T) {
	store := mustNewVideoStore(t)
	store.Create("abc123", "Test Video", 100, "0xCreator")
	store.SetReady("abc123", "tb", "https://agg/v1/blobs/tb", "pb", "https://agg/v1/blobs/pb", "fb", "https://agg/v1/blobs/fb")
	store.SetSuiObjectID("abc123", "0xOBJ999", "encBlob", "https://agg/v1/blobs/encBlob")

	h := NewStreamFull(store)
	req := httptest.NewRequest(http.MethodGet, "/stream/0xOBJ999/full", nil)
	req.SetPathValue("id", "0xOBJ999")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307, got %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "https://agg/v1/blobs/encBlob" {
		t.Errorf("expected redirect to encrypted blob URL, got %s", got)
	}
}

func TestStreamFull_PayLockID_RedirectsToCanonical(t *testing.T) {
	store := mustNewVideoStore(t)
	store.Create("abc123", "Test Video", 100, "0xCreator")
	store.SetReady("abc123", "tb", "https://agg/v1/blobs/tb", "pb", "https://agg/v1/blobs/pb", "fb", "https://agg/v1/blobs/fb")
	store.SetSuiObjectID("abc123", "0xOBJ999", "encBlob", "https://agg/v1/blobs/encBlob")

	h := NewStreamFull(store)
	req := httptest.NewRequest(http.MethodGet, "/stream/abc123/full", nil)
	req.SetPathValue("id", "abc123")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307, got %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/stream/0xOBJ999/full" {
		t.Errorf("expected canonical redirect to /stream/0xOBJ999/full, got %s", got)
	}
}

