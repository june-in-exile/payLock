# AGENTS.md

Instructions for agentic coding agents working in this repository.

## Project Overview

Orca is a video-aware middleware layer for HLS streaming. It accepts MP4 uploads, segments them into HLS via FFmpeg, and serves them with byte-range and CORS support. Built in Go with the standard library `net/http`.

## Build Commands

```bash
make run          # Run dev server (go run ./cmd/orca)
make build        # Compile to bin/orca
make test         # Run all tests with race detector and coverage
make lint         # go vet ./...
make clean        # Remove bin/ and storage/
```

### Running a Single Test

```bash
# Single test file
go test ./internal/middleware/ -v

# Single test function
go test ./internal/processor/... -run TestValidateMagicBytes -v
```

## Prerequisites

- Go 1.25+
- `ffmpeg` and `ffprobe` must be installed and on `PATH`

## Code Style Guidelines

### General

- No external dependencies beyond Go standard library (except external FFmpeg binaries)
- Use `log/slog` for structured logging (not `log`)
- Prefer early returns to reduce nesting
- Keep functions focused and small

### Naming Conventions

- **Packages**: lowercase, single word or short phrase (e.g., `handler`, `middleware`)
- **Types**: PascalCase (e.g., `VideoStore`, `Upload`, `Processor`)
- **Functions**: PascalCase for exported, camelCase for unexported
- **Interfaces**: noun-based, singular (e.g., `Backend`, not `StorageBackend`)
- **Constants**: PascalCase for exported, camelCase for unexported
- **Variables**: camelCase; use short names for local scope
- **Error variables**: `Err` prefix (e.g., `ErrInvalidFormat`)

### Imports

Group imports in this order:

1. Standard library (no prefix)
2. Third-party packages (empty line before)
3. Internal packages with full import path (empty line before)

```go
import (
    "context"
    "encoding/json"
    "net/http"

    "github.com/anthropics/orca/internal/config"
)
```

### Error Handling

- Define sentinel errors with `errors.New` or `fmt.Errorf` with `%w`
- Wrap errors with context: `fmt.Errorf("action: %w", err)`
- Return errors early; avoid `else` after error checks
- Use `errors.Is` / `errors.As` for error inspection

```go
// Good
if err != nil {
    slog.Error("action failed", "id", id, "error", err)
    return fmt.Errorf("action: %w", err)
}

// For user-facing errors
writeJSON(w, http.StatusBadRequest, map[string]string{
    "error": "specific user message",
})
```

### Structs and Types

- Use embedded structs for composition
- Use struct tags for JSON serialization (e.g., `json:"id"`)
- Private fields start with lowercase
- Consider `sync.RWMutex` for concurrent access patterns

```go
type Video struct {
    ID        string  `json:"id"`
    Status    Status  `json:"status"`
    CreatedAt string  `json:"created_at"`
    Duration  float64 `json:"duration,omitempty"`
    Error     string  `json:"error,omitempty"`
}

type VideoStore struct {
    mu     sync.RWMutex
    videos map[string]*Video
}
```

### HTTP Handlers

- Implement `http.Handler` interface (ServeHTTP method)
- Use `http.HandlerFunc` adapter for simple handlers
- Set headers before writing status code
- Use `http.ServeFile` for static file serving (handles Range requests)
- Wrap handlers with middleware using adapter pattern

```go
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(data)
}

func Middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // pre-processing
        next.ServeHTTP(w, r)
        // post-processing (if needed)
    })
}
```

### Context and Concurrency

- Pass `context.Context` for cancellation and timeouts
- Use goroutines for background work; capture loop variables properly
- Prefer `sync.RWMutex` over `sync.Mutex` when reads dominate
- Always call `defer cancel()` when creating derived contexts

```go
go func() {
    h.processVideo(id, filePath)
}()

ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
```

### Testing

- Test files: `*_test.go` in same package
- Use `httptest` for HTTP handler tests
- Use table-driven tests for multiple cases
- Test error paths explicitly

```go
func TestValidateMagicBytes(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {"valid mp4", "....ftyp", false},
        {"invalid", "....xxxx", true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test logic
        })
    }
}
```

### Security Considerations

- Validate file uploads with magic bytes (MP4: `ftyp` at offset 4)
- Prevent path traversal with strict filename validation
- Limit request body size with `http.MaxBytesReader`
- Use constant-time comparison for API keys (for this codebase, simple comparison is acceptable)

## Environment Variables

| Var | Default | Description |
|-----|---------|-------------|
| `ORCA_PORT` | `8080` | HTTP listen port |
| `ORCA_STORAGE_DIR` | `./storage` | Local storage root |
| `ORCA_API_KEY` | _(none)_ | Required for `/api/*` endpoints |
| `ORCA_FFMPEG_PATH` | `ffmpeg` | Path to ffmpeg binary |
| `ORCA_FFPROBE_PATH` | `ffprobe` | Path to ffprobe binary |
| `ORCA_MAX_FILE_SIZE_MB` | `500` | Upload size limit in MB |

## Directory Structure

```
cmd/orca/main.go          — Entry point; wires all packages
internal/config/          — Environment-based configuration
internal/model/           — Data models (Video, VideoStore)
internal/storage/         — Backend interface + LocalStorage implementation
internal/processor/       — FFmpeg wrapper (Segment/Probe) + validators
internal/handler/         — HTTP handlers (upload, status, stream)
internal/middleware/     — APIKey auth + CORS middleware
```
