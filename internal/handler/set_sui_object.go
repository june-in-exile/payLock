package handler

import (
	"encoding/json"
	"net/http"

	"github.com/anthropics/orca/internal/model"
)

type SetSuiObject struct {
	videos *model.VideoStore
}

func NewSetSuiObject(videos *model.VideoStore) *SetSuiObject {
	return &SetSuiObject{videos: videos}
}

func (h *SetSuiObject) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "missing video id",
		})
		return
	}

	var body struct {
		SuiObjectID string `json:"sui_object_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
		return
	}

	if body.SuiObjectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "sui_object_id is required",
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

	if video.SuiObjectID != "" {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "sui object id already set",
		})
		return
	}

	if !h.videos.SetSuiObjectID(id, body.SuiObjectID) {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "video not found",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":        "ok",
		"sui_object_id": body.SuiObjectID,
	})
}
