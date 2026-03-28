# PayLock Project Context

## Project Overview

PayLock is a **video-native decentralized storage infrastructure** for Sui. It is transitioning from a v1 video-native middleware (local processing/streaming) to a v2 architecture centered around **Walrus** (decentralized storage) and **Seal** (access control).

## Technologies

- **Language**: Go (v1.25+)
- **Storage**: Walrus (Testnet)
- **Video Processing**: FFmpeg (required for previews and thumbnails)
- **Architecture**:
  - `cmd/paylock`: Server entry point.
  - `internal/handler`: HTTP handlers (Upload/Status/Videos). Integrates with Walrus.
  - `internal/walrus`: Client for Walrus Publisher and Aggregator.
  - `internal/model`: In-memory state with disk persistence for video metadata and Walrus Blob IDs.
  - `internal/watcher`: Polls Sui for `VideoCreated` events to link local records.
  - `internal/indexer`: On-startup reindexing from chain.

## Core Workflows

1. **Upload (`POST /api/upload`)**:
   - Receives video via multipart form.
   - **Free Videos**: Server extracts a preview (if needed), optimizes for fast-start, and uploads both preview and full video to Walrus.
   - **Paid Videos**: Server extracts a preview and thumbnail, uploads them to Walrus, and waits for the client to encrypt and upload the full video on-chain.
2. **Status (`GET /api/status/{id}`)**:
   - SSE endpoint providing real-time updates on processing and Walrus upload progress.
3. **On-chain Link (`PATCH /api/videos/{id}/link`)**:
   - After `create_video` succeeds on-chain, the client calls this endpoint with `sui_object_id` and `full_blob_id` to immediately link the on-chain object and transition the video to `ready`.
4. **Chain Sync (fallback)**:
   - Background watcher monitors Sui events to link on-chain video objects to local metadata as a fallback if the PATCH call fails.

## Development Conventions

- **Walrus First**: All video data resides on Walrus.
- **Asynchronous Operations**: Heavy I/O and processing (FFmpeg, Walrus uploads) happen in background goroutines.
- **Metadata Coordinator**: The backend acts as a coordinator between the client, Walrus, and Sui, rather than a heavy streaming proxy.

## Environment Variables

- `PAYLOCK_WALRUS_PUBLISHER_URL`: Walrus publisher endpoint.
- `PAYLOCK_WALRUS_AGGREGATOR_URL`: Walrus aggregator endpoint.
- `PAYLOCK_WALRUS_EPOCHS`: Number of epochs to store blobs on Walrus (default 5).
- `PAYLOCK_ENABLE_FFMPEG`: Enable FFmpeg processing for previews and thumbnails (default true).
- `PAYLOCK_GATING_PACKAGE_ID`: Deployed gating Move package ID on Sui.
- `PAYLOCK_SUI_RPC_URL`: Sui RPC node URL.
- `PAYLOCK_DATA_DIR`: Directory for persistent video metadata (default "data").
