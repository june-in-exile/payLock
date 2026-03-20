package handler

import (
	"net/http"
	"path/filepath"
	"regexp"

	"github.com/anthropics/orca/internal/model"
	"github.com/anthropics/orca/internal/storage"
)

var validFilename = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

type Stream struct {
	store  storage.Backend
	videos *model.VideoStore
}

func NewStream(store storage.Backend, videos *model.VideoStore) *Stream {
	return &Stream{store: store, videos: videos}
}

func (h *Stream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	file := r.PathValue("file")

	if id == "" || file == "" {
		http.Error(w, "missing id or file", http.StatusBadRequest)
		return
	}

	// Prevent path traversal
	if !validFilename.MatchString(file) {
		http.Error(w, "invalid filename", http.StatusBadRequest)
		return
	}

	// Check video exists and is ready
	video, ok := h.videos.Get(id)
	if !ok {
		http.Error(w, "video not found", http.StatusNotFound)
		return
	}
	if video.Status != model.StatusReady {
		http.Error(w, "video is not ready", http.StatusServiceUnavailable)
		return
	}

	// Resolve file path
	var filePath string
	var err error

	ext := filepath.Ext(file)
	switch ext {
	case ".m3u8":
		filePath, err = h.store.ManifestPath(id)
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-store")
	case ".ts":
		filePath, err = h.store.SegmentPath(id, file)
		w.Header().Set("Content-Type", "video/mp2t")
	case ".mp4":
		filePath, err = h.store.SegmentPath(id, file)
		w.Header().Set("Content-Type", "video/mp4")
	default:
		http.Error(w, "unsupported file type", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	// http.ServeFile handles Range requests, Content-Length, etc.
	http.ServeFile(w, r, filePath)
}
