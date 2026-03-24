package handler

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/anthropics/paylock/internal/config"
	"github.com/anthropics/paylock/internal/model"
	"github.com/anthropics/paylock/internal/processor"
)

// Storer abstracts Walrus blob storage for testability.
type Storer interface {
	Store(data []byte, epochs int) (string, error)
	BlobURL(blobID string) string
}

type Upload struct {
	walrus Storer
	videos *model.VideoStore
	cfg    *config.Config
}

func NewUpload(w Storer, videos *model.VideoStore, cfg *config.Config) *Upload {
	return &Upload{walrus: w, videos: videos, cfg: cfg}
}

func (h *Upload) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.cfg.MaxFileSize)

	data, title, price, creator, err := h.parseRequest(r)
	if err != nil {
		writeJSON(w, err.status, map[string]string{"error": err.msg})
		return
	}

	id := generateID()
	if title == "" {
		title = id
	}

	h.videos.Create(id, title, price, creator)

	go h.processAndUpload(id, data)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"id":     id,
		"status": model.StatusProcessing,
	})
}

type requestError struct {
	status int
	msg    string
}

func (h *Upload) parseRequest(r *http.Request) ([]byte, string, uint64, string, *requestError) {
	file, header, err := r.FormFile("video")
	if err != nil {
		return nil, "", 0, "", &requestError{http.StatusBadRequest, "failed to read video file: " + err.Error()}
	}
	defer file.Close()

	if err := processor.ValidateSize(header.Size, h.cfg.MaxFileSize); err != nil {
		return nil, "", 0, "", &requestError{http.StatusRequestEntityTooLarge, err.Error()}
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, "", 0, "", &requestError{http.StatusBadRequest, "failed to read video file"}
	}

	if err := processor.ValidateMagicBytes(bytes.NewReader(data)); err != nil {
		return nil, "", 0, "", &requestError{http.StatusBadRequest, "invalid file format: supported formats are MP4, MOV, WebM, MKV, AVI"}
	}

	title := r.FormValue("title")
	creator := r.FormValue("creator")

	var price uint64
	if v := r.FormValue("price"); v != "" {
		price, err = strconv.ParseUint(v, 10, 64)
		if err != nil {
			return nil, "", 0, "", &requestError{http.StatusBadRequest, "invalid price: must be a positive integer (MIST)"}
		}
	}

	return data, title, price, creator, nil
}

func (h *Upload) processAndUpload(id string, data []byte) {
	video, ok := h.videos.Get(id)
	if !ok {
		return
	}

	previewData, err := processor.ExtractPreview(data, h.cfg.PreviewDuration, h.cfg.FFmpegPath)
	if err != nil {
		slog.Error("preview extraction failed", "id", id, "error", err)
		h.videos.SetFailed(id, "preview extraction failed: "+err.Error())
		return
	}

	thumbnailData, err := processor.ExtractThumbnail(data, h.cfg.FFmpegPath)
	if err != nil {
		slog.Warn("thumbnail extraction failed, continuing without thumbnail", "id", id, "error", err)
	}

	// Paid videos: upload only preview + thumbnail; full blob is encrypted + uploaded by the frontend
	if video.Price > 0 {
		thumbBlobID, thumbBlobURL := h.uploadThumbnail(id, thumbnailData)

		previewBlobID, err := h.walrus.Store(previewData, h.cfg.WalrusEpochs)
		if err != nil {
			slog.Error("walrus upload failed", "id", id, "error", err)
			h.videos.SetFailed(id, "upload to Walrus failed: "+err.Error())
			return
		}
		previewBlobURL := h.walrus.BlobURL(previewBlobID)
		h.videos.SetReady(id, thumbBlobID, thumbBlobURL, previewBlobID, previewBlobURL, "", "")
		slog.Info("preview uploaded to walrus (paid video, awaiting encrypted full blob)",
			"id", id,
			"preview_blob_id", previewBlobID,
		)
		return
	}

	// Free videos: upload thumbnail, preview, and full blobs
	thumbBlobID, thumbBlobURL := h.uploadThumbnail(id, thumbnailData)

	fastData, err := processor.EnsureFaststart(data, h.cfg.FFmpegPath)
	if err != nil {
		slog.Warn("faststart failed, uploading original", "id", id, "error", err)
		fastData = data
	}

	previewBlobID, fullBlobID, err := h.uploadBothBlobs(previewData, fastData)
	if err != nil {
		slog.Error("walrus upload failed", "id", id, "error", err)
		h.videos.SetFailed(id, "upload to Walrus failed: "+err.Error())
		return
	}

	previewBlobURL := h.walrus.BlobURL(previewBlobID)
	fullBlobURL := h.walrus.BlobURL(fullBlobID)
	h.videos.SetReady(id, thumbBlobID, thumbBlobURL, previewBlobID, previewBlobURL, fullBlobID, fullBlobURL)
	slog.Info("video uploaded to walrus",
		"id", id,
		"preview_blob_id", previewBlobID,
		"full_blob_id", fullBlobID,
	)
}

// uploadThumbnail uploads thumbnail data to Walrus. Returns empty strings if thumbnail is nil.
func (h *Upload) uploadThumbnail(id string, thumbnailData []byte) (string, string) {
	if len(thumbnailData) == 0 {
		return "", ""
	}
	blobID, err := h.walrus.Store(thumbnailData, h.cfg.WalrusEpochs)
	if err != nil {
		slog.Warn("thumbnail upload failed", "id", id, "error", err)
		return "", ""
	}
	return blobID, h.walrus.BlobURL(blobID)
}

// uploadBothBlobs uploads preview and full data to Walrus in parallel.
func (h *Upload) uploadBothBlobs(preview, full []byte) (string, string, error) {
	type result struct {
		blobID string
		err    error
	}

	previewCh := make(chan result, 1)
	fullCh := make(chan result, 1)

	go func() {
		blobID, err := h.walrus.Store(preview, h.cfg.WalrusEpochs)
		previewCh <- result{blobID, err}
	}()
	go func() {
		blobID, err := h.walrus.Store(full, h.cfg.WalrusEpochs)
		fullCh <- result{blobID, err}
	}()

	pr := <-previewCh
	fr := <-fullCh

	if pr.err != nil {
		return "", "", pr.err
	}
	if fr.err != nil {
		return "", "", fr.err
	}
	return pr.blobID, fr.blobID, nil
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
