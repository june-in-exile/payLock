package handler

import (
	"log/slog"
	"net/http"

	"github.com/anthropics/orca/internal/model"
	"github.com/anthropics/orca/internal/storage"
)

type Delete struct {
	store  *storage.LocalStorage
	videos *model.VideoStore
}

func NewDelete(store *storage.LocalStorage, videos *model.VideoStore) *Delete {
	return &Delete{store: store, videos: videos}
}

func (h *Delete) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "missing video id",
		})
		return
	}

	if _, ok := h.videos.Get(id); !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "video not found",
		})
		return
	}

	if err := h.store.Delete(id); err != nil {
		slog.Error("failed to delete video files", "id", id, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to delete video files",
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
