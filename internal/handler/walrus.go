package handler

import (
	"fmt"
	"io"
	"net/http"
	"regexp"

	"github.com/anthropics/orca/internal/config"
)

var validBlobID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type WalrusBlob struct {
	aggregatorURL string
}

func NewWalrusBlob(cfg *config.Config) *WalrusBlob {
	return &WalrusBlob{aggregatorURL: cfg.WalrusAggregator}
}

func (h *WalrusBlob) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.aggregatorURL == "" {
		http.Error(w, "Walrus not configured", http.StatusServiceUnavailable)
		return
	}

	blobID := r.PathValue("blobId")
	if blobID == "" || !validBlobID.MatchString(blobID) {
		http.Error(w, "invalid blob ID", http.StatusBadRequest)
		return
	}

	url := fmt.Sprintf("%s/v1/blobs/%s", h.aggregatorURL, blobID)

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
	if err != nil {
		http.Error(w, "failed to create request", http.StatusInternalServerError)
		return
	}

	// Forward Range header if present
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "failed to fetch blob: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Forward important headers
	for k, v := range resp.Header {
		if k == "Content-Type" || k == "Content-Length" || k == "Cache-Control" || k == "Content-Range" || k == "Accept-Ranges" {
			w.Header().Set(k, v[0])
		}
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
