package handler

import (
	"net/http"

	"github.com/anthropics/paylock/internal/config"
)

type AppConfig struct {
	cfg *config.Config
}

func NewAppConfig(cfg *config.Config) *AppConfig {
	return &AppConfig{cfg: cfg}
}

func (h *AppConfig) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"gating_package_id":     h.cfg.GatingPackageID,
		"sui_network":           "testnet",
		"walrus_publisher_url":  h.cfg.WalrusPublisher,
		"walrus_aggregator_url": h.cfg.WalrusAggregator,
		"preview_duration":      h.cfg.PreviewDuration,
	})
}
