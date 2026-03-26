package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/anthropics/paylock/internal/model"
)

type StatusEvents struct {
	videos *model.VideoStore
}

func NewStatusEvents(videos *model.VideoStore) *StatusEvents {
	return &StatusEvents{videos: videos}
}

func (h *StatusEvents) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing video id", http.StatusBadRequest)
		return
	}

	// Atomically resolve the video and subscribe if still processing.
	// This eliminates the race between checking status and subscribing.
	ch, video, _, found := h.videos.ResolveAndSubscribeIfProcessing(id)
	if !found {
		http.NotFound(w, r)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// If already terminal, send one event and close.
	if ch == nil {
		writeSSEEvent(w, flusher, video)
		return
	}

	// Send initial processing event.
	writeSSEEvent(w, flusher, video)

	// Wait for status change.
	select {
	case v := <-ch:
		writeSSEEvent(w, flusher, &v)
	case <-r.Context().Done():
		// Client disconnected.
	}
}

func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, video *model.Video) {
	data, err := json.Marshal(video)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}
