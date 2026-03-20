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
	"github.com/anthropics/orca/internal/walrus"
)

//go:embed web
var webFS embed.FS

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if err := processor.CheckFFmpeg(cfg.FFmpegPath); err != nil {
		slog.Error("ffmpeg is required but not found", "error", err)
		os.Exit(1)
	}

	wc := walrus.NewClient(cfg.WalrusPublisher, cfg.WalrusAggregator)
	videos := model.NewVideoStore()

	mux := http.NewServeMux()

	// API routes
	mux.Handle("POST /api/upload", handler.NewUpload(wc, videos, cfg))
	mux.Handle("GET /api/status/{id}", handler.NewStatus(videos))
	mux.Handle("GET /api/videos", handler.NewVideos(videos))
	mux.Handle("DELETE /api/videos/{id}", handler.NewDelete(videos))

	// Stream routes — redirect to Walrus aggregator
	cors := middleware.CORS()
	mux.Handle("GET /stream/{id}", cors(handler.NewStream(videos)))
	mux.Handle("GET /stream/{id}/full", cors(handler.NewStreamFull(videos)))

	// Frontend (embedded static files)
	webSub, err := fs.Sub(webFS, "web")
	if err != nil {
		slog.Error("failed to create web filesystem", "error", err)
		os.Exit(1)
	}
	fileServer := http.FileServer(http.FS(webSub))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path != "/" {
			_, err := fs.Stat(webSub, path[1:])
			if err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("orca server starting",
			"port", cfg.Port,
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
