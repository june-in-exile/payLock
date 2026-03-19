package handler

import (
	"fmt"
	"io"
	"net/http"

	"github.com/anthropics/orca/internal/config"
)

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
	if blobID == "" {
		http.Error(w, "missing blob ID", http.StatusBadRequest)
		return
	}

	url := fmt.Sprintf("%s/v1/blobs/%s", h.aggregatorURL, blobID)

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
	if err != nil {
		http.Error(w, "failed to create request", http.StatusInternalServerError)
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "failed to fetch blob: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		http.Error(w, fmt.Sprintf("walrus error %d: %s", resp.StatusCode, string(body)), resp.StatusCode)
		return
	}

	for k, v := range resp.Header {
		if k == "Content-Type" || k == "Content-Length" || k == "Cache-Control" {
			w.Header().Set(k, v[0])
		}
	}

	w.WriteHeader(http.StatusOK)
	io.Copy(w, resp.Body)
}
