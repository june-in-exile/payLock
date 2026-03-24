package handler

import (
	"log/slog"
	"net/http"

	"github.com/anthropics/paylock/internal/model"
)

type Delete struct {
	videos *model.VideoStore
}

func NewDelete(videos *model.VideoStore) *Delete {
	return &Delete{videos: videos}
}

func (h *Delete) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "missing video id",
		})
		return
	}

	video, ok := h.videos.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "video not found",
		})
		return
	}

	if !verifyCreator(video, r.Header.Get(creatorHeader)) {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "forbidden: X-Creator does not match video creator",
		})
		return
	}

	h.videos.Delete(id)
	slog.Info("video deleted", "id", id)
	writeJSON(w, http.StatusOK, map[string]string{
		"id":     id,
		"status": "deleted",
	})
}
