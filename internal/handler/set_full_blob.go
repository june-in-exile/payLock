package handler

import (
	"encoding/json"
	"net/http"

	"github.com/anthropics/orca/internal/model"
)

type SetFullBlob struct {
	walrus Storer
	videos *model.VideoStore
}

func NewSetFullBlob(w Storer, videos *model.VideoStore) *SetFullBlob {
	return &SetFullBlob{walrus: w, videos: videos}
}

func (h *SetFullBlob) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "missing video id",
		})
		return
	}

	var body struct {
		FullBlobID string `json:"full_blob_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
		return
	}

	if body.FullBlobID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "full_blob_id is required",
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

	if video.FullBlobID != "" {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "full blob id already set",
		})
		return
	}

	fullBlobURL := h.walrus.BlobURL(body.FullBlobID)
	if !h.videos.SetFullBlob(id, body.FullBlobID, fullBlobURL) {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "video not found",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":        "ok",
		"full_blob_id":  body.FullBlobID,
		"full_blob_url": fullBlobURL,
	})
}
