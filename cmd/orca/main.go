package main

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/anthropics/orca/internal/config"
	"github.com/anthropics/orca/internal/handler"
	"github.com/anthropics/orca/internal/middleware"
	"github.com/anthropics/orca/internal/model"
	"github.com/anthropics/orca/internal/processor"
	"github.com/anthropics/orca/internal/storage"
)

//go:embed web
var webFS embed.FS

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	store := storage.NewLocal(cfg.StorageDir)
	proc := processor.New(cfg.FFmpegPath, cfg.FFprobePath)
	videos := model.NewVideoStore()

	mux := http.NewServeMux()

	// API routes (with API key auth)
	apiAuth := middleware.APIKey(cfg.APIKey)
	mux.Handle("POST /api/upload", apiAuth(handler.NewUpload(store, proc, videos, cfg)))
	mux.Handle("GET /api/status/{id}", apiAuth(handler.NewStatus(videos)))

	// Video list
	mux.Handle("GET /api/videos", apiAuth(handler.NewVideos(videos)))
	mux.Handle("GET /api/walrus/{blobId}", handler.NewWalrusBlob(cfg))

	// Stream routes (with CORS)
	cors := middleware.CORS()
	mux.Handle("GET /stream/{id}/{file...}", cors(handler.NewStream(store, videos)))

	// Frontend (embedded static files)
	webSub, err := fs.Sub(webFS, "web")
	if err != nil {
		slog.Error("failed to create web filesystem", "error", err)
		os.Exit(1)
	}
	mux.Handle("GET /", http.FileServer(http.FS(webSub)))

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("orca server starting",
			"port", cfg.Port,
			"storage", cfg.StorageDir,
			"walrus_publisher", cfg.WalrusPublisher,
			"walrus_aggregator", cfg.WalrusAggregator,
		)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}

	slog.Info("server stopped")
}
