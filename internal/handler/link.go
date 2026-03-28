package handler

import (
	"encoding/json"
	"net/http"

	"github.com/anthropics/paylock/internal/model"
)

type linkRequest struct {
	SuiObjectID string `json:"sui_object_id"`
	FullBlobID  string `json:"full_blob_id"`
}

type Link struct {
	videos  *model.VideoStore
	walrus  Storer
}

func NewLink(videos *model.VideoStore, walrus Storer) *Link {
	return &Link{videos: videos, walrus: walrus}
}

func (h *Link) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing video id"})
		return
	}

	var req linkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.SuiObjectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sui_object_id is required"})
		return
	}

	fullBlobURL := ""
	if req.FullBlobID != "" {
		fullBlobURL = h.walrus.BlobURL(req.FullBlobID)
	}

	if !h.videos.SetSuiObjectID(id, req.SuiObjectID, req.FullBlobID, fullBlobURL) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "video not found"})
		return
	}

	video, ok := h.videos.Get(id)
	if !ok {
		video, ok = h.videos.GetBySuiObjectID(req.SuiObjectID)
	}
	if !ok {
		writeJSON(w, http.StatusOK, map[string]string{"status": "linked"})
		return
	}

	writeJSON(w, http.StatusOK, video)
}
