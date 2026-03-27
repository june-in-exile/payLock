# Repo Split Plan (Frontend + PayLock Infra)

## Goal

Split the current codebase into two repositories:

- **Infra repo**: `paylock` (blob proxy + chain sync + stream redirects)
- **Frontend repo**: application layer UI + video processing (ffmpeg.wasm)

This document captures the planned updates for the current repo **before** implementation.

## Key Decisions

- Infra will be public and usable by any frontend.
- `/api/*` endpoints in infra must support CORS for cross-origin frontends.
- Final state is **two separate repos**, not a monorepo with two folders.
- Sui Move contracts (`contracts/`) stay in the infra repo — they define the on-chain protocol.
- **FFmpeg moves to frontend** — server never processes video content, only validates and relays blobs. This ensures the full unencrypted video never reaches the server (paid flow) and keeps the infra layer generic.

## Current Architecture

```
cmd/paylock/
├── main.go            — wires all packages; embeds SPA via go:embed
└── web/               — embedded SPA (11 vanilla JS files)

internal/
├── config/            — env-based configuration
├── handler/           — HTTP handlers (upload, status, videos, delete, config, stream)
├── indexer/           — Sui chain reindexer (FetchAll on startup)
├── middleware/         — CORS middleware (currently /stream/* only)
├── model/             — VideoStore (sync.RWMutex + JSON file persistence)
├── processor/         — FFmpeg validators, magic-byte checks, preview extraction
├── suiauth/           — Sui wallet signature verification
├── testutil/          — test helpers
├── walrus/            — Walrus HTTP client (Store, BlobURL)
└── watcher/           — chain event watcher (polls VideoCreated events)

contracts/             — Sui Move contract (gating.move)
```

### Current Routes

| Method | Path | CORS | Description |
|--------|------|------|-------------|
| `POST` | `/api/upload` | No | Upload video (202 async) |
| `GET` | `/api/status/{id}` | No | Get video status |
| `GET` | `/api/status/{id}/events` | No | SSE stream for status updates |
| `GET` | `/api/videos` | No | List videos (paginated) |
| `DELETE` | `/api/videos/{id}` | No | Delete video record |
| `GET` | `/api/config` | No | Client configuration |
| `GET` | `/stream/{id}/preview` | Yes | 307 redirect to Walrus preview blob |
| `GET` | `/stream/{id}/full` | Yes | 307 redirect to Walrus full blob |
| `GET` | `/` | — | Serve embedded SPA (fallback routing) |

## Target Architecture (After Split)

```
Frontend (ffmpeg.wasm)                  PayLock Infra (no FFmpeg)
──────────────────────                  ────────────────────────
1. User selects video
2. ffmpeg.wasm produces:
   - preview clip (MP4)
   - thumbnail (JPEG)
   - faststart full blob (free only)
3. (paid) Seal-encrypt full blob
   → upload encrypted blob to Walrus

4. POST /api/upload ──────────────────→ 5. Validate magic bytes + size
   fields: preview, thumbnail,          6. Upload blobs to Walrus
   full (free only), title, price       7. Return 202 + id
                                        8. Chain watcher syncs on-chain state
```

### Infra responsibilities (= what any frontend integrates with)

| Layer | Responsibility |
|-------|---------------|
| **Blob proxy** | Validate format (magic bytes) + size limits → store to Walrus |
| **Metadata** | VideoStore + JSON persistence + chain sync (indexer + watcher) |
| **Stream** | `/stream/{id}/*` → 307 redirect to Walrus aggregator |
| **Auth** | Sui wallet signature verification |

### Upload API (new contract)

```
POST /api/upload (multipart/form-data)

Free (price = 0):
  preview    file, required   — frontend-generated preview clip
  thumbnail  file, optional   — frontend-generated JPEG thumbnail
  full       file, required   — frontend-processed (faststart) full video
  title      string, optional
  price      "0" or omitted

Paid (price > 0):
  preview    file, required   — frontend-generated preview clip
  thumbnail  file, optional   — frontend-generated JPEG thumbnail
  title      string, optional
  price      string, required — price in MIST
  (full blob: frontend encrypts via Seal and uploads directly to Walrus)
```

## Scope of Changes (Infra Repo)

### Code Changes

1. **Remove embedded frontend SPA**
   - SPA assets live in `cmd/paylock/web/` (11 files: app.js, wallet.js, player-view.js, upload-section.js, etc.)
   - Server embeds SPA via `//go:embed web` in `cmd/paylock/main.go`
   - Plan: delete `cmd/paylock/web/`, remove `go:embed` directive, remove the `GET /` SPA handler (lines 106-124)
   - Replace `GET /` with a simple JSON health/info response (e.g., `{"service": "paylock", "version": "..."}`)

2. **Remove FFmpeg dependency**
   - Delete `internal/processor/ffmpeg.go` (ExtractPreview, ExtractThumbnail, EnsureFaststart, ValidatePreviewDuration, CheckFFmpeg)
   - Delete `internal/processor/ffmpeg_test.go`
   - Remove FFmpeg config fields from `internal/config/config.go` (`FFmpegEnabled`, `FFmpegPath`, `FFprobePath`, preview duration settings)
   - Remove env vars: `PAYLOCK_ENABLE_FFMPEG`, `PAYLOCK_MAX_PREVIEW_DURATION`
   - Remove FFmpeg startup check from `cmd/paylock/main.go` (lines 35-42)
   - Keep: `internal/processor/validator.go` (magic-byte validation, size validation — these remain server-side)

3. **Refactor upload handler**
   - `handleFreeUpload`: accept `preview` + `thumbnail` + `full` as separate file fields (no server-side extraction)
   - `handlePaidUpload`: accept `preview` + `thumbnail` only (no `video` field — full blob never reaches server)
   - Remove `processAndUpload` / `processAndUploadPaid` FFmpeg pipelines
   - Replace with: validate magic bytes → upload blobs to Walrus → done
   - Async flow (202 + poll) still applies for the Walrus upload step

4. **CORS for `/api/*`**
   - Current state: only `/stream/*` has CORS (allows `Origin: *`, methods `GET, OPTIONS`, header `Range`)
   - Plan: enable CORS for all `/api/*` routes
   - Required methods: `GET, POST, DELETE, OPTIONS`
   - Required headers: `Content-Type, Range, X-Wallet-Address, X-Wallet-Sig, X-Wallet-Timestamp, X-Creator`
   - Expose headers: `Content-Range, Content-Length`
   - Allowed origins: configurable via `PAYLOCK_CORS_ORIGINS` env var (default: `*` for dev, restrict in prod)

5. **SSE CORS consideration**
   - `GET /api/status/{id}/events` uses Server-Sent Events — needs CORS to work cross-origin
   - EventSource API only sends simple requests, so standard CORS headers suffice

### Documentation Changes

1. **README.md**
   - Remove "embedded web UI" and "FFmpeg preview extraction" from features list
   - Remove SPA-related and FFmpeg setup instructions
   - Clarify infra as standalone blob proxy + chain sync service
   - Add section on integrating with an external frontend
   - Document new upload contract (frontend provides all blobs)

2. **API.md**
   - Update `POST /api/upload` to reflect new multipart fields (preview, thumbnail, full)
   - Add CORS documentation (allowed origins, preflight behavior)
   - Reframe examples as "external frontend integration"

3. **Agent docs (CLAUDE.md, GEMINI.md, AGENTS.md)**
   - Remove `cmd/paylock/web/` from architecture descriptions
   - Remove FFmpeg references from architecture and env var tables
   - Update route table (remove `GET /`, add health endpoint)

## Scope of Changes (Frontend Repo)

- Create new repo with its own README
- Migrate `cmd/paylock/web/` files as starting point
- Integrate `ffmpeg.wasm` for client-side video processing:
  - Preview clip extraction (first N seconds)
  - Thumbnail generation (first frame → JPEG)
  - Faststart optimization (moov atom relocation)
- Document setup, env vars (e.g., `PAYLOCK_BASE_URL` for API base)
- Include wallet integration flows (Sui wallet sig auth)
- Document Seal encryption flow (client-side encrypt → Walrus upload)
- Document purchase flow (AccessPass → Seal decrypt → playback)
- Add build/dev tooling as needed (currently vanilla JS, no bundler)

## Non-Goals

- No refactors to chain indexer/watcher logic
- No changes to Sui Move contracts
- No new features beyond FFmpeg removal, CORS, and repo split cleanup

## Resolved Decisions

- **`GET /` behavior**: return a JSON health/info response (not 404 or redirect)
- **CORS policy**: configurable via env var; default `*` for development convenience
- **Contracts location**: stays in infra repo (defines the on-chain protocol)
- **FFmpeg**: moves entirely to frontend (ffmpeg.wasm); server does not process video
- **Preview duration validation**: not enforced server-side; frontend/contract responsibility

## Open Questions

- Should the frontend repo use a bundler/framework, or stay vanilla JS?
- Do we need a shared types/constants package between repos (e.g., auth header names)?

## Proposed Order of Work

1. Remove embedded SPA and `GET /` route; add health endpoint
2. Remove FFmpeg dependency and refactor upload handler to accept pre-processed blobs
3. Add `/api/*` CORS support (with configurable origins)
4. Update infra docs (README, API.md, agent docs)
5. Create frontend repo, migrate UI assets, integrate ffmpeg.wasm
6. Verify cross-origin integration works end-to-end
