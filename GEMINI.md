# PayLock Project Context

## Project Overview

PayLock is a **video-native decentralized storage infrastructure** for Sui. It is transitioning from a v1 video-native middleware (local processing/streaming) to a v2 architecture centered around **Walrus** (decentralized storage) and **Seal** (access control).

## Technologies

- **Language**: Go (v1.25+)
- **Storage**: Walrus (Testnet)
- **Video Processing**: FFmpeg (optional; required for paid uploads to generate previews)
- **Architecture**:
  - `cmd/paylock`: Server entry point.
  - `internal/handler`: HTTP handlers (Upload/Status/Videos). Now integrates with Walrus.
  - `internal/walrus`: Client for Walrus Publisher and Aggregator.
  - `internal/model`: In-memory state for video metadata and Walrus Blob IDs.
  - `internal/config`: Configuration for Walrus endpoints and local settings.

## Core Workflows (Current v2 Transition)

1. **Upload**: 
   - Receives MP4 via `POST /api/upload`.
   - Validates magic bytes (MP4) and file size.
   - **New**: Uploads the raw video blob directly to Walrus asynchronously.
2. **Streaming**: 
   - `GET /stream/{id}` is currently a proxy/redirect layer pointing to the Walrus Aggregator.
3. **Status**: 
   - Monitors the upload progress to Walrus and returns the `blobId` and `blobUrl`.

## Development Conventions

- **Walrus First**: All video data should eventually reside on Walrus.
- **Asynchronous Operations**: Heavy I/O (like Walrus uploads) happens in background goroutines.
- **Minimal Middleware**: Moving away from being a "heavy" proxy towards being a metadata/access coordinator.

## Environment Variables

- `PAYLOCK_WALRUS_PUBLISHER_URL`: Walrus publisher endpoint.
- `PAYLOCK_WALRUS_AGGREGATOR_URL`: Walrus aggregator endpoint.
- `PAYLOCK_WALRUS_EPOCHS`: Number of epochs to store blobs on Walrus.
- `PAYLOCK_ENABLE_FFMPEG`: Enable FFmpeg processing for previews and thumbnails (default true).
- `PAYLOCK_GATING_PACKAGE_ID`: Deployed gating Move package ID on Sui.
