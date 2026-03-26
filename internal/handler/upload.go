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
	"github.com/anthropics/paylock/internal/suiauth"
)

// Storer abstracts Walrus blob storage for testability.
type Storer interface {
	Store(data []byte, epochs int) (string, error)
	BlobURL(blobID string) string
}

type Upload struct {
	walrus   Storer
	videos   *model.VideoStore
	cfg      *config.Config
	verifier SigVerifier
	clock    suiauth.Clock
}

func NewUpload(w Storer, videos *model.VideoStore, cfg *config.Config, verifier SigVerifier, clock suiauth.Clock) *Upload {
	return &Upload{walrus: w, videos: videos, cfg: cfg, verifier: verifier, clock: clock}
}

func (h *Upload) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Apply the larger limit initially so we can parse the price field.
	// The paid path re-applies a tighter limit via MaxPreviewSize after branching.
	r.Body = http.MaxBytesReader(w, r.Body, h.cfg.MaxFileSize)

	price, err := parsePriceField(r)
	if err != nil {
		writeJSON(w, err.status, map[string]string{"error": err.msg})
		return
	}

	if price > 0 {
		h.handlePaidUpload(w, r, price)
	} else {
		h.handleFreeUpload(w, r)
	}
}

type requestError struct {
	status int
	msg    string
}

func parsePriceField(r *http.Request) (uint64, *requestError) {
	v := r.FormValue("price")
	if v == "" {
		return 0, nil
	}
	price, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0, &requestError{http.StatusBadRequest, "invalid price: must be a positive integer (MIST)"}
	}
	return price, nil
}

func parsePreviewDuration(r *http.Request, cfg *config.Config) (int, *requestError) {
	v := r.FormValue("preview_duration")
	if v == "" {
		return cfg.PreviewDurationDefault, nil
	}
	sec, err := strconv.Atoi(v)
	if err != nil {
		return 0, &requestError{http.StatusBadRequest, "invalid preview_duration: must be an integer number of seconds"}
	}
	if sec < cfg.MinPreviewDuration || sec > cfg.MaxPreviewDuration {
		return 0, &requestError{http.StatusBadRequest, "invalid preview_duration: must be between " + strconv.Itoa(cfg.MinPreviewDuration) + " and " + strconv.Itoa(cfg.MaxPreviewDuration) + " seconds"}
	}
	return sec, nil
}

func (h *Upload) handleFreeUpload(w http.ResponseWriter, r *http.Request) {
	data, title, reqErr := h.parseFreeRequest(r)
	if reqErr != nil {
		writeJSON(w, reqErr.status, map[string]string{"error": reqErr.msg})
		return
	}

	previewDuration, reqErr := parsePreviewDuration(r, h.cfg)
	if reqErr != nil {
		writeJSON(w, reqErr.status, map[string]string{"error": reqErr.msg})
		return
	}

	// Optionally extract creator from wallet auth headers (not required for free uploads).
	var creator string
	if auth := extractAndVerifyWalletAuth(r, h.verifier, h.clock, "upload", ""); auth.err == "" {
		creator = auth.address
	}

	id := generateID()
	if title == "" {
		title = id
	}

	h.videos.Create(id, title, 0, creator)

	go h.processAndUpload(id, data, previewDuration)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"id":     id,
		"status": model.StatusProcessing,
	})
}

func (h *Upload) handlePaidUpload(w http.ResponseWriter, r *http.Request, price uint64) {
	auth := extractAndVerifyWalletAuth(r, h.verifier, h.clock, "upload", "")
	if auth.err != "" {
		writeJSON(w, auth.status, map[string]string{"error": auth.err})
		return
	}

	if !h.cfg.FFmpegEnabled {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "paid uploads require ffmpeg/ffprobe to validate preview duration"})
		return
	}

	if _, reqErr := parsePreviewDuration(r, h.cfg); reqErr != nil {
		writeJSON(w, reqErr.status, map[string]string{"error": reqErr.msg})
		return
	}

	previewData, thumbnailData, title, reqErr := h.parsePaidRequest(r)
	if reqErr != nil {
		writeJSON(w, reqErr.status, map[string]string{"error": reqErr.msg})
		return
	}

	id := generateID()
	if title == "" {
		title = id
	}

	h.videos.Create(id, title, price, auth.address)

	go h.processAndUploadPaid(id, previewData, thumbnailData)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"id":     id,
		"status": model.StatusProcessing,
	})
}

func (h *Upload) parseFreeRequest(r *http.Request) ([]byte, string, *requestError) {
	file, header, err := r.FormFile("video")
	if err != nil {
		return nil, "", &requestError{http.StatusBadRequest, "failed to read video file: " + err.Error()}
	}
	defer file.Close()

	if err := processor.ValidateSize(header.Size, h.cfg.MaxFileSize); err != nil {
		return nil, "", &requestError{http.StatusRequestEntityTooLarge, err.Error()}
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, "", &requestError{http.StatusBadRequest, "failed to read video file"}
	}

	if err := processor.ValidateMagicBytes(bytes.NewReader(data)); err != nil {
		return nil, "", &requestError{http.StatusBadRequest, "invalid file format: supported formats are MP4, MOV, WebM, MKV, AVI"}
	}

	return data, r.FormValue("title"), nil
}

func (h *Upload) parsePaidRequest(r *http.Request) ([]byte, []byte, string, *requestError) {
	file, header, err := r.FormFile("preview")
	if err != nil {
		return nil, nil, "", &requestError{http.StatusBadRequest, "paid uploads require a 'preview' field (short video clip generated client-side)"}
	}
	defer file.Close()

	if err := processor.ValidateSize(header.Size, h.cfg.MaxPreviewSize); err != nil {
		return nil, nil, "", &requestError{http.StatusRequestEntityTooLarge, err.Error()}
	}

	previewData, err := io.ReadAll(file)
	if err != nil {
		return nil, nil, "", &requestError{http.StatusBadRequest, "failed to read preview file"}
	}

	if err := processor.ValidateMagicBytes(bytes.NewReader(previewData)); err != nil {
		return nil, nil, "", &requestError{http.StatusBadRequest, "invalid preview format: supported formats are MP4, MOV, WebM, MKV, AVI"}
	}

	var thumbnailData []byte
	if thumbFile, _, thumbErr := r.FormFile("thumbnail"); thumbErr == nil {
		defer thumbFile.Close()
		thumbData, readErr := io.ReadAll(thumbFile)
		if readErr != nil {
			return nil, nil, "", &requestError{http.StatusBadRequest, "failed to read thumbnail file"}
		}
		if err := processor.ValidateJPEGMagicBytes(bytes.NewReader(thumbData)); err != nil {
			return nil, nil, "", &requestError{http.StatusBadRequest, "invalid thumbnail format: expected JPEG"}
		}
		thumbnailData = thumbData
	}

	return previewData, thumbnailData, r.FormValue("title"), nil
}

// processAndUploadPaid handles async processing for paid video uploads.
// The preview and thumbnail are already validated and provided by the frontend.
func (h *Upload) processAndUploadPaid(id string, previewData, thumbnailData []byte) {
	if h.cfg.FFmpegEnabled {
		if err := processor.ValidatePreviewDuration(previewData, h.cfg.MaxPreviewDuration, h.cfg.FFprobePath); err != nil {
			slog.Error("preview duration validation failed", "id", id, "error", err)
			h.videos.SetFailed(id, err.Error())
			return
		}
	}

	thumbBlobID, thumbBlobURL := h.uploadThumbnail(id, thumbnailData)

	previewBlobID, err := h.walrus.Store(previewData, h.cfg.WalrusEpochs)
	if err != nil {
		slog.Error("walrus upload failed", "id", id, "error", err)
		h.videos.SetFailed(id, "upload to Walrus failed: "+err.Error())
		return
	}
	previewBlobURL := h.walrus.BlobURL(previewBlobID)
	h.videos.SetPreviewUploaded(id, thumbBlobID, thumbBlobURL, previewBlobID, previewBlobURL)
	slog.Info("preview uploaded to walrus (paid video, awaiting encrypted full blob)",
		"id", id,
		"preview_blob_id", previewBlobID,
	)
}

// processAndUpload handles async processing for free video uploads.
func (h *Upload) processAndUpload(id string, data []byte, previewDuration int) {
	previewData := data
	var thumbnailData []byte
	if h.cfg.FFmpegEnabled {
		var err error
		previewData, err = processor.ExtractPreview(data, previewDuration, h.cfg.FFmpegPath)
		if err != nil {
			slog.Error("preview extraction failed", "id", id, "error", err)
			h.videos.SetFailed(id, "preview extraction failed: "+err.Error())
			return
		}

		thumbnailData, err = processor.ExtractThumbnail(data, h.cfg.FFmpegPath)
		if err != nil {
			slog.Warn("thumbnail extraction failed, continuing without thumbnail", "id", id, "error", err)
		}
	} else {
		slog.Info("ffmpeg disabled; using full file as preview", "id", id)
	}

	thumbBlobID, thumbBlobURL := h.uploadThumbnail(id, thumbnailData)

	fastData := data
	if h.cfg.FFmpegEnabled {
		var err error
		fastData, err = processor.EnsureFaststart(data, h.cfg.FFmpegPath)
		if err != nil {
			slog.Warn("faststart failed, uploading original", "id", id, "error", err)
			fastData = data
		}
	}

	if !h.cfg.FFmpegEnabled || bytes.Equal(previewData, fastData) {
		fullBlobID, err := h.walrus.Store(fastData, h.cfg.WalrusEpochs)
		if err != nil {
			slog.Error("walrus upload failed", "id", id, "error", err)
			h.videos.SetFailed(id, "upload to Walrus failed: "+err.Error())
			return
		}
		fullBlobURL := h.walrus.BlobURL(fullBlobID)
		h.videos.SetReady(id, thumbBlobID, thumbBlobURL, fullBlobID, fullBlobURL, fullBlobID, fullBlobURL)
		slog.Info("video uploaded to walrus (single blob)",
			"id", id,
			"full_blob_id", fullBlobID,
		)
		return
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
