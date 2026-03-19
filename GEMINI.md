# Orca Project Context

## Project Overview

Orca is a **video-native decentralized storage protocol** implemented in Go. It acts as a middleware layer on top of general-purpose storage (like Walrus or local storage), handling video-specific logic such as HLS (HTTP Live Streaming) segmentation, manifest management, and metadata indexing. The goal is to make decentralized video streaming as seamless as traditional CDNs.

## Technologies

- **Language**: Go (v1.25+)
- **Video Processing**: FFmpeg & FFprobe
- **Streaming Protocol**: HLS (m3u8/ts)
- **Architecture**:
  - `cmd/orca`: Entry point for the server.
  - `internal/handler`: HTTP handlers for upload, status, and streaming.
  - `internal/processor`: Wrapper for FFmpeg/FFprobe operations.
  - `internal/storage`: Abstraction for video storage (currently supports local disk).
  - `internal/model`: In-memory state management for video metadata.
  - `internal/middleware`: API Key authentication and CORS support.

## Building and Running

The project includes a `Makefile` for common tasks:

- **Run development server**: `make run`
- **Build binary**: `make build` (Output: `bin/orca`)
- **Run tests**: `make test`
- **Lint code**: `make lint`
- **Clean workspace**: `make clean` (Removes `bin/` and `storage/`)

## Environment Variables

Configure the server using the following environment variables:

- `ORCA_PORT`: Port to listen on (default: `8080`).
- `ORCA_STORAGE_DIR`: Path to the directory for storing videos (default: `./storage`).
- `ORCA_API_KEY`: Required API key for management operations (Upload/Status).
- `ORCA_MAX_FILE_SIZE_MB`: Max upload size in megabytes (default: `500`).
- `ORCA_FFMPEG_PATH`: Path to `ffmpeg` executable (default: `ffmpeg`).
- `ORCA_FFPROBE_PATH`: Path to `ffprobe` executable (default: `ffprobe`).

## API Endpoints

- `POST /api/upload`: Upload an MP4 video (multipart form, key: `video`). Returns a video ID. (Requires `ORCA_API_KEY`)
- `GET /api/status/{id}`: Check the processing status of a video. (Requires `ORCA_API_KEY`)
- `GET /stream/{id}/index.m3u8`: HLS manifest for playback.
- `GET /stream/{id}/{segment}.ts`: Individual video segments.

## Development Conventions

- **Surgical Updates**: Prefer clean, idiomatic Go using the standard library where possible.
- **Asynchronous Processing**: Uploads are acknowledged immediately; video segmentation and validation happen in background goroutines.
- **Logging**: Use `log/slog` for structured logging.
- **Validation**: Strict validation of magic bytes (MP4) and file size before accepting uploads.
- **Errors**: Return JSON error responses for all API failures.
