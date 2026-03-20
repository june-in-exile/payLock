package handler

import (
	"net/http"

	"github.com/anthropics/orca/internal/config"
)

type AppConfig struct {
	cfg *config.Config
}

func NewAppConfig(cfg *config.Config) *AppConfig {
	return &AppConfig{cfg: cfg}
}

func (h *AppConfig) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"paywall_package_id": h.cfg.PaywallPackageID,
	})
}
