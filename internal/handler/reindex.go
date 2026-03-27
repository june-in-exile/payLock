package handler

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"

	"github.com/anthropics/paylock/internal/indexer"
	"github.com/anthropics/paylock/internal/model"
)

type Reindex struct {
	indexer     *indexer.Indexer
	videos      *model.VideoStore
	blobURL     func(blobID string) string
	adminSecret string
}

func NewReindex(idx *indexer.Indexer, videos *model.VideoStore, blobURL func(string) string, adminSecret string) *Reindex {
	return &Reindex{indexer: idx, videos: videos, blobURL: blobURL, adminSecret: adminSecret}
}

// ServeHTTP triggers a full chain reindex and returns the result.
// Requires Authorization: Bearer <PAYLOCK_ADMIN_SECRET> header.
func (h *Reindex) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "unauthorized: invalid or missing admin secret",
		})
		return
	}

	chainVideos, err := h.indexer.FetchAll(r.Context())
	if err != nil {
		slog.Error("reindex failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "reindex failed: " + err.Error(),
		})
		return
	}

	existing := make(map[string]struct{}, len(chainVideos))
	created := 0
	for _, cv := range chainVideos {
		existing[cv.ObjectID] = struct{}{}
		thumbnailURL := h.blobURL(cv.ThumbnailBlobID)
		previewURL := h.blobURL(cv.PreviewBlobID)
		fullURL := h.blobURL(cv.FullBlobID)

		if h.videos.UpsertFromChain(cv.ObjectID, cv.Title, cv.Price, cv.Creator, cv.ThumbnailBlobID, thumbnailURL, cv.PreviewBlobID, previewURL, cv.FullBlobID, fullURL) {
			created++
		}
	}

	pruned := h.videos.PruneMissingChain(existing)
	slog.Info("reindex complete", "chain_total", len(chainVideos), "new_entries", created, "pruned", pruned)

	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"chain_total": len(chainVideos),
		"new_entries": created,
		"pruned":      pruned,
	})
}

func (h *Reindex) authorize(r *http.Request) bool {
	if h.adminSecret == "" {
		return false
	}
	auth := r.Header.Get("Authorization")
	token, ok := strings.CutPrefix(auth, "Bearer ")
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(h.adminSecret)) == 1
}
