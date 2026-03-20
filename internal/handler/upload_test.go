package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/anthropics/orca/internal/config"
	"github.com/anthropics/orca/internal/model"
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
		MaxFileSize:     500 * 1024 * 1024,
		WalrusEpochs:    1,
		FFmpegPath:      "ffmpeg",
		PreviewDuration: 2,
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

func TestUpload_InvalidFormat(t *testing.T) {
	store := &mockStorer{storeFunc: func(data []byte, epochs int) (string, error) {
		return "blob1", nil
	}}
	videos := model.NewVideoStore()
	h := NewUpload(store, videos, newTestConfig())

	req := createMultipartRequest(t, "video", "test.txt", []byte("not an mp4 file content here!!"), nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestUpload_ValidMP4_Accepted(t *testing.T) {
	mp4Data, err := os.ReadFile("../../test.mp4")
	if err != nil {
		t.Fatalf("failed to read test.mp4: %v", err)
	}

	var callCount atomic.Int32
	store := &mockStorer{storeFunc: func(data []byte, epochs int) (string, error) {
		n := callCount.Add(1)
		return fmt.Sprintf("blob%d", n), nil
	}}
	videos := model.NewVideoStore()
	h := NewUpload(store, videos, newTestConfig())

	req := createMultipartRequest(t, "video", "test.mp4", mp4Data, map[string]string{
		"title":   "Test Video",
		"price":   "100000000",
		"creator": "0xCAFE",
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
	mp4Data, err := os.ReadFile("../../test.mp4")
	if err != nil {
		t.Fatalf("failed to read test.mp4: %v", err)
	}

	store := &mockStorer{storeFunc: func(data []byte, epochs int) (string, error) {
		return "blob1", nil
	}}
	videos := model.NewVideoStore()
	h := NewUpload(store, videos, newTestConfig())

	req := createMultipartRequest(t, "video", "test.mp4", mp4Data, map[string]string{
		"price": "not-a-number",
	})
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestUpload_MissingFile(t *testing.T) {
	store := &mockStorer{storeFunc: func(data []byte, epochs int) (string, error) {
		return "blob1", nil
	}}
	videos := model.NewVideoStore()
	h := NewUpload(store, videos, newTestConfig())

	req := httptest.NewRequest(http.MethodPost, "/api/upload", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
