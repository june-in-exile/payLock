package handler

import (
	"net/http"

	"github.com/anthropics/orca/internal/model"
)

type Stream struct {
	videos *model.VideoStore
}

func NewStream(videos *model.VideoStore) *Stream {
	return &Stream{videos: videos}
}

// ServeHTTP redirects to the preview blob URL (public, anyone can access).
func (h *Stream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing video id", http.StatusBadRequest)
		return
	}

	video, ok := h.videos.Get(id)
	if !ok {
		http.Error(w, "video not found", http.StatusNotFound)
		return
	}
	if video.Status != model.StatusReady {
		http.Error(w, "video is not ready", http.StatusServiceUnavailable)
		return
	}
	if video.PreviewBlobURL == "" {
		http.Error(w, "video has no blob URL", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, video.PreviewBlobURL, http.StatusTemporaryRedirect)
}

type StreamFull struct {
	videos *model.VideoStore
}

func NewStreamFull(videos *model.VideoStore) *StreamFull {
	return &StreamFull{videos: videos}
}

// ServeHTTP redirects to the full blob URL.
// In Phase 2, this will require Seal decryption on the frontend.
func (h *StreamFull) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing video id", http.StatusBadRequest)
		return
	}

	video, ok := h.videos.Get(id)
	if !ok {
		http.Error(w, "video not found", http.StatusNotFound)
		return
	}
	if video.Status != model.StatusReady {
		http.Error(w, "video is not ready", http.StatusServiceUnavailable)
		return
	}
	if video.FullBlobURL == "" {
		http.Error(w, "video has no full blob URL", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, video.FullBlobURL, http.StatusTemporaryRedirect)
}
