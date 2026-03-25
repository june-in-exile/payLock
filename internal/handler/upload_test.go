package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anthropics/paylock/internal/config"
	"github.com/anthropics/paylock/internal/model"
	"github.com/anthropics/paylock/internal/suiauth"
	"github.com/anthropics/paylock/internal/testutil"
)

type mockStorer struct {
	storeFunc func(data []byte, epochs int) (string, error)
}

func (m *mockStorer) Store(data []byte, epochs int) (string, error) {
	return m.storeFunc(data, epochs)
}

func (m *mockStorer) BlobURL(blobID string) string {
	return "https://agg/v1/blobs/" + blobID
}

func newTestConfig() *config.Config {
	return &config.Config{
		MaxFileSize:        500 * 1024 * 1024,
		MaxPreviewSize:     50 * 1024 * 1024,
		MaxPreviewDuration: 30,
		WalrusEpochs:       1,
		FFmpegEnabled:      false,
		FFmpegPath:         "ffmpeg",
		PreviewDuration:    2,
	}
}

func createMultipartRequest(t *testing.T, fieldName, filename string, data []byte, fields map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	part, err := w.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	part.Write(data)

	for k, v := range fields {
		w.WriteField(k, v)
	}
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/upload", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func createPaidMultipartRequest(t *testing.T, previewData, thumbnailData []byte, fields map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	part, err := w.CreateFormFile("preview", "preview.mp4")
	if err != nil {
		t.Fatalf("create preview form file: %v", err)
	}
	part.Write(previewData)

	if thumbnailData != nil {
		thumbPart, err := w.CreateFormFile("thumbnail", "thumb.jpg")
		if err != nil {
			t.Fatalf("create thumbnail form file: %v", err)
		}
		thumbPart.Write(thumbnailData)
	}

	for k, v := range fields {
		w.WriteField(k, v)
	}
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/upload", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func TestUpload_InvalidFormat(t *testing.T) {
	store := &mockStorer{storeFunc: func(data []byte, epochs int) (string, error) {
		return "blob1", nil
	}}
	videos := mustNewVideoStore(t)
	h := NewUpload(store, videos, newTestConfig(), nil, nil)

	req := createMultipartRequest(t, "video", "test.txt", []byte("not an mp4 file content here!!"), nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestUpload_ValidMP4_Accepted(t *testing.T) {
	mp4Data := testutil.TestMP4(t)

	var callCount atomic.Int32
	store := &mockStorer{storeFunc: func(data []byte, epochs int) (string, error) {
		n := callCount.Add(1)
		return fmt.Sprintf("blob%d", n), nil
	}}
	videos := mustNewVideoStore(t)
	h := NewUpload(store, videos, newTestConfig(), nil, nil)

	req := createMultipartRequest(t, "video", "test.mp4", mp4Data, map[string]string{
		"title": "Test Video",
	})
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["status"] != string(model.StatusProcessing) {
		t.Errorf("expected status processing, got %v", resp["status"])
	}
	if resp["id"] == nil || resp["id"] == "" {
		t.Error("expected non-empty id")
	}
}

func TestUpload_InvalidPrice(t *testing.T) {
	mp4Data := testutil.TestMP4(t)

	store := &mockStorer{storeFunc: func(data []byte, epochs int) (string, error) {
		return "blob1", nil
	}}
	videos := mustNewVideoStore(t)
	h := NewUpload(store, videos, newTestConfig(), nil, nil)

	req := createMultipartRequest(t, "video", "test.mp4", mp4Data, map[string]string{
		"price": "not-a-number",
	})
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestUpload_PaidWithoutCreator(t *testing.T) {
	store := &mockStorer{storeFunc: func(data []byte, epochs int) (string, error) {
		return "blob1", nil
	}}
	videos := mustNewVideoStore(t)
	h := NewUpload(store, videos, newTestConfig(), nil, nil)

	mp4Data := testutil.TestMP4(t)
	req := createMultipartRequest(t, "preview", "preview.mp4", mp4Data, map[string]string{
		"price": "100000000",
	})
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	// Paid upload without wallet auth headers should get 401
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for paid upload without auth, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpload_PaidPreview_Accepted(t *testing.T) {
	mp4Data := testutil.TestMP4(t)

	var callCount atomic.Int32
	store := &mockStorer{storeFunc: func(data []byte, epochs int) (string, error) {
		n := callCount.Add(1)
		return fmt.Sprintf("blob%d", n), nil
	}}
	videos := mustNewVideoStore(t)
	v := &mockVerifier{address: "0xAlice", err: nil}
	h := NewUpload(store, videos, newTestConfig(), v, suiauth.FixedClock(time.Now().Unix()))

	req := createPaidMultipartRequest(t, mp4Data, nil, map[string]string{
		"price": "100000000",
		"title": "Paid Video",
	})
	setAuthHeaders(req, "0xAlice", "fakesig", nowTS())
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != string(model.StatusProcessing) {
		t.Errorf("expected status processing, got %v", resp["status"])
	}
}

func TestUpload_PaidPreview_MissingPreviewField(t *testing.T) {
	mp4Data := testutil.TestMP4(t)

	store := &mockStorer{storeFunc: func(data []byte, epochs int) (string, error) {
		return "blob1", nil
	}}
	videos := mustNewVideoStore(t)
	v := &mockVerifier{address: "0xAlice", err: nil}
	h := NewUpload(store, videos, newTestConfig(), v, suiauth.FixedClock(time.Now().Unix()))

	// Send "video" field instead of "preview" — should fail
	req := createMultipartRequest(t, "video", "test.mp4", mp4Data, map[string]string{
		"price": "100000000",
	})
	setAuthHeaders(req, "0xAlice", "fakesig", nowTS())
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing preview field, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpload_PaidPreview_InvalidFormat(t *testing.T) {
	store := &mockStorer{storeFunc: func(data []byte, epochs int) (string, error) {
		return "blob1", nil
	}}
	videos := mustNewVideoStore(t)
	v := &mockVerifier{address: "0xAlice", err: nil}
	h := NewUpload(store, videos, newTestConfig(), v, suiauth.FixedClock(time.Now().Unix()))

	req := createPaidMultipartRequest(t, []byte("not a valid video format!!"), nil, map[string]string{
		"price": "100000000",
	})
	setAuthHeaders(req, "0xAlice", "fakesig", nowTS())
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid preview format, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpload_PaidPreview_PreviewTooLarge(t *testing.T) {
	store := &mockStorer{storeFunc: func(data []byte, epochs int) (string, error) {
		return "blob1", nil
	}}
	videos := mustNewVideoStore(t)
	cfg := newTestConfig()
	cfg.MaxPreviewSize = 100 // 100 bytes — tiny limit for test
	v := &mockVerifier{address: "0xAlice", err: nil}
	h := NewUpload(store, videos, cfg, v, suiauth.FixedClock(time.Now().Unix()))

	mp4Data := testutil.TestMP4(t)
	req := createPaidMultipartRequest(t, mp4Data, nil, map[string]string{
		"price": "100000000",
	})
	setAuthHeaders(req, "0xAlice", "fakesig", nowTS())
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for preview too large, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpload_PaidPreview_WithThumbnail(t *testing.T) {
	mp4Data := testutil.TestMP4(t)
	// Minimal JPEG: FF D8 FF E0 + some bytes
	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46}

	var callCount atomic.Int32
	store := &mockStorer{storeFunc: func(data []byte, epochs int) (string, error) {
		n := callCount.Add(1)
		return fmt.Sprintf("blob%d", n), nil
	}}
	videos := mustNewVideoStore(t)
	v := &mockVerifier{address: "0xAlice", err: nil}
	h := NewUpload(store, videos, newTestConfig(), v, suiauth.FixedClock(time.Now().Unix()))

	req := createPaidMultipartRequest(t, mp4Data, jpegData, map[string]string{
		"price": "100000000",
		"title": "Paid With Thumb",
	})
	setAuthHeaders(req, "0xAlice", "fakesig", nowTS())
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpload_PaidPreview_InvalidThumbnail(t *testing.T) {
	mp4Data := testutil.TestMP4(t)

	store := &mockStorer{storeFunc: func(data []byte, epochs int) (string, error) {
		return "blob1", nil
	}}
	videos := mustNewVideoStore(t)
	v := &mockVerifier{address: "0xAlice", err: nil}
	h := NewUpload(store, videos, newTestConfig(), v, suiauth.FixedClock(time.Now().Unix()))

	req := createPaidMultipartRequest(t, mp4Data, []byte("not a jpeg"), map[string]string{
		"price": "100000000",
	})
	setAuthHeaders(req, "0xAlice", "fakesig", nowTS())
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid thumbnail, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpload_PaidPreview_NoFFmpegRequired(t *testing.T) {
	mp4Data := testutil.TestMP4(t)

	var callCount atomic.Int32
	store := &mockStorer{storeFunc: func(data []byte, epochs int) (string, error) {
		n := callCount.Add(1)
		return fmt.Sprintf("blob%d", n), nil
	}}
	videos := mustNewVideoStore(t)
	cfg := newTestConfig()
	cfg.FFmpegEnabled = false
	v := &mockVerifier{address: "0xAlice", err: nil}
	h := NewUpload(store, videos, cfg, v, suiauth.FixedClock(time.Now().Unix()))

	req := createPaidMultipartRequest(t, mp4Data, nil, map[string]string{
		"price": "100000000",
	})
	setAuthHeaders(req, "0xAlice", "fakesig", nowTS())
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 even without ffmpeg, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpload_FreeUpload_StillUsesVideoField(t *testing.T) {
	mp4Data := testutil.TestMP4(t)

	var callCount atomic.Int32
	store := &mockStorer{storeFunc: func(data []byte, epochs int) (string, error) {
		n := callCount.Add(1)
		return fmt.Sprintf("blob%d", n), nil
	}}
	videos := mustNewVideoStore(t)
	h := NewUpload(store, videos, newTestConfig(), nil, nil)

	req := createMultipartRequest(t, "video", "test.mp4", mp4Data, map[string]string{
		"title": "Free Video",
	})
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for free upload with video field, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpload_MissingFile(t *testing.T) {
	store := &mockStorer{storeFunc: func(data []byte, epochs int) (string, error) {
		return "blob1", nil
	}}
	videos := mustNewVideoStore(t)
	h := NewUpload(store, videos, newTestConfig(), nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/upload", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
