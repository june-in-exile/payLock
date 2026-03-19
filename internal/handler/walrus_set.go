package handler

import (
	"encoding/json"
	"net/http"

	"github.com/anthropics/orca/internal/model"
)

type WalrusSet struct {
	videos *model.VideoStore
}

func NewWalrusSet(videos *model.VideoStore) *WalrusSet {
	return &WalrusSet{videos: videos}
}

func (h *WalrusSet) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	var body struct {
		BlobID string `json:"blob_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
		return
	}

	if body.BlobID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "blob_id is required",
		})
		return
	}

	h.videos.SetWalrusBlobID(id, body.BlobID)

	writeJSON(w, http.StatusOK, map[string]string{
		"id":      id,
		"blob_id": body.BlobID,
	})
}
