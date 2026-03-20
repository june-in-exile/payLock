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

func scanStorage(store *storage.LocalStorage, videos *model.VideoStore) {
	ids, err := store.List()
	if err != nil {
		slog.Warn("failed to scan storage directory", "error", err)
		return
	}
	for _, id := range ids {
		dir := store.OutputDir(id)
		info, err := os.Stat(dir)
		var t time.Time
		if err == nil {
			t = info.ModTime().UTC()
		} else {
			t = time.Now().UTC()
		}

		title := id
		if meta, err := store.LoadMetadata(id); err == nil && meta.Title != "" {
			title = meta.Title
		}

		if store.HasManifest(id) {
			videos.Restore(id, title, model.StatusReady, t)
			slog.Info("restored video", "id", id, "title", title, "status", "ready")
		} else if store.HasUpload(id) {
			videos.Restore(id, title, model.StatusFailed, t)
			slog.Info("restored video", "id", id, "title", title, "status", "failed")
		}
	}
	if len(ids) > 0 {
		slog.Info("restored videos from storage", "count", len(ids))
	}
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	store := storage.NewLocal(cfg.StorageDir)

	proc := processor.New(cfg.FFmpegPath, cfg.FFprobePath)
	videos := model.NewVideoStore()

	scanStorage(store, videos)

	mux := http.NewServeMux()

	// API routes
	mux.Handle("POST /api/upload", handler.NewUpload(store, proc, videos, cfg))
	mux.Handle("GET /api/status/{id}", handler.NewStatus(videos))
	mux.Handle("GET /api/videos", handler.NewVideos(videos))
	mux.Handle("DELETE /api/videos/{id}", handler.NewDelete(store, videos))

	// Stream routes (with CORS)
	cors := middleware.CORS()
	mux.Handle("GET /stream/{id}/{file...}", cors(handler.NewStream(store, videos)))

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
			"storage_dir", cfg.StorageDir,
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
