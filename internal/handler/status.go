package handler

import (
	"net/http"

	"github.com/anthropics/paylock/internal/model"
)

type Status struct {
	videos *model.VideoStore
}

func NewStatus(videos *model.VideoStore) *Status {
	return &Status{videos: videos}
}

func (h *Status) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "missing video id",
		})
		return
	}

	video, ok := h.videos.Get(id)
	if !ok {
		video, ok = h.videos.GetBySuiObjectID(id)
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "video not found",
		})
		return
	}

	writeJSON(w, http.StatusOK, video)
}
