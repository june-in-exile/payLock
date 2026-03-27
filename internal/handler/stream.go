package handler

import (
	"net/http"

	"github.com/anthropics/paylock/internal/model"
)

type StreamPreview struct {
	videos *model.VideoStore
}

func NewStreamPreview(videos *model.VideoStore) *StreamPreview {
	return &StreamPreview{videos: videos}
}

// ServeHTTP redirects to the preview blob URL (public, anyone can access).
// Supports both paylock_id and sui_object_id lookups.
// When accessed by paylock_id and the video has a sui_object_id, returns 307 to the canonical URL.
func (h *StreamPreview) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing video id", http.StatusBadRequest)
		return
	}

	video, canonical, ok := h.videos.Resolve(id)
	if !ok {
		http.Error(w, "video not found", http.StatusNotFound)
		return
	}

	// If accessed by paylock_id and a canonical sui_object_id exists, redirect.
	if !canonical && video.SuiObjectID != "" {
		canonicalURL := "/stream/" + video.SuiObjectID + "/preview"
		http.Redirect(w, r, canonicalURL, http.StatusTemporaryRedirect)
		return
	}

	// Deprecation warning when accessed by paylock_id.
	if !canonical {
		setDeprecationHeaders(w, "/stream/{sui_object_id}/preview")
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
// Supports both paylock_id and sui_object_id lookups.
// When accessed by paylock_id and the video has a sui_object_id, returns 307 to the canonical URL.
func (h *StreamFull) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing video id", http.StatusBadRequest)
		return
	}

	video, canonical, ok := h.videos.Resolve(id)
	if !ok {
		http.Error(w, "video not found", http.StatusNotFound)
		return
	}

	// If accessed by paylock_id and a canonical sui_object_id exists, redirect.
	if !canonical && video.SuiObjectID != "" {
		canonicalURL := "/stream/" + video.SuiObjectID + "/full"
		http.Redirect(w, r, canonicalURL, http.StatusTemporaryRedirect)
		return
	}

	// Deprecation warning when accessed by paylock_id.
	if !canonical {
		setDeprecationHeaders(w, "/stream/{sui_object_id}/full")
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


// setDeprecationHeaders adds standard deprecation headers to warn clients
// that the paylock_id-based path is deprecated in favor of sui_object_id.
func setDeprecationHeaders(w http.ResponseWriter, canonical string) {
	w.Header().Set("Deprecation", "true")
	w.Header().Set("Sunset", "2026-06-23")
	w.Header().Set("Link", `<`+canonical+`>; rel="successor-version"`)
}
