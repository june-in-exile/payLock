package handler

import (
	"log/slog"
	"net/http"

	"github.com/anthropics/paylock/internal/model"
	"github.com/anthropics/paylock/internal/suiauth"
)

type Delete struct {
	videos   *model.VideoStore
	verifier SigVerifier
	clock    suiauth.Clock
}

func NewDelete(videos *model.VideoStore, verifier SigVerifier, clock suiauth.Clock) *Delete {
	return &Delete{videos: videos, verifier: verifier, clock: clock}
}

func (h *Delete) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "missing video id",
		})
		return
	}

	video, _, ok := h.videos.Resolve(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "video not found",
		})
		return
	}

	// Videos not yet published on-chain can be deleted without auth (cleanup of failed uploads).
	// Once on-chain, require creator signature.
	if video.Creator != "" && video.SuiObjectID != "" {
		auth := extractAndVerifyWalletAuth(r, h.verifier, h.clock, "delete", id)
		if auth.err != "" {
			writeJSON(w, auth.status, map[string]string{"error": auth.err})
			return
		}
		if !verifyOwnership(video, auth.address) {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "forbidden: wallet address does not match video creator",
			})
			return
		}
	}

	h.videos.Delete(video.ID)
	slog.Info("video deleted", "id", video.ID)
	writeJSON(w, http.StatusOK, map[string]string{
		"id":     video.ID,
		"status": "deleted",
	})
}
