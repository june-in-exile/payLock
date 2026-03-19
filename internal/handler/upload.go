package handler

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/anthropics/orca/internal/config"
	"github.com/anthropics/orca/internal/model"
	"github.com/anthropics/orca/internal/processor"
	"github.com/anthropics/orca/internal/storage"
	"github.com/anthropics/orca/internal/walrus"
)

type Upload struct {
	store  storage.Backend
	proc   *processor.Processor
	videos *model.VideoStore
	cfg    *config.Config
	walrus *walrus.Client
}

func NewUpload(store storage.Backend, proc *processor.Processor, videos *model.VideoStore, cfg *config.Config) *Upload {
	var wc *walrus.Client
	if cfg.WalrusPublisher != "" && cfg.WalrusAggregator != "" {
		wc = walrus.NewClient(cfg.WalrusPublisher, cfg.WalrusAggregator)
	}
	return &Upload{store: store, proc: proc, videos: videos, cfg: cfg, walrus: wc}
}

func (h *Upload) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.cfg.MaxFileSize)

	file, header, err := r.FormFile("video")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "failed to read video file: " + err.Error(),
		})
		return
	}
	defer file.Close()

	if err := processor.ValidateSize(header.Size, h.cfg.MaxFileSize); err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{
			"error": err.Error(),
		})
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "failed to read video file",
		})
		return
	}

	if err := processor.ValidateMagicBytes(bytes.NewReader(data)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid file format: only MP4 files are accepted",
		})
		return
	}

	id := generateID()

	filePath, err := h.store.SaveUpload(id, bytes.NewReader(data))
	if err != nil {
		slog.Error("failed to save upload", "id", id, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to save video",
		})
		return
	}

	h.videos.Create(id)

	go h.processVideo(id, filePath)

	if h.walrus != nil {
		go h.uploadToWalrus(id, data)
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"id":     id,
		"status": model.StatusProcessing,
	})
}

func (h *Upload) processVideo(id, filePath string) {
	ctx := context.Background()

	duration, err := h.proc.Probe(filePath)
	if err != nil {
		slog.Error("ffprobe validation failed", "id", id, "error", err)
		h.videos.SetFailed(id, "video validation failed: "+err.Error())
		return
	}

	outputDir := h.store.OutputDir(id)
	if err := h.proc.Segment(ctx, filePath, outputDir); err != nil {
		slog.Error("ffmpeg segmentation failed", "id", id, "error", err)
		h.videos.SetFailed(id, "video processing failed: "+err.Error())
		return
	}

	h.videos.SetReady(id, duration)
	slog.Info("video ready", "id", id, "duration", duration)
}

func (h *Upload) uploadToWalrus(id string, data []byte) {
	blobID, err := h.walrus.Store(data)
	if err != nil {
		slog.Error("walrus upload failed", "id", id, "error", err)
		return
	}
	h.videos.SetWalrusBlobID(id, blobID)
	slog.Info("walrus upload succeeded", "id", id, "blob_id", blobID)
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
