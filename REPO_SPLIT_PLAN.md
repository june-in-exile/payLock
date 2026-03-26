# Repo Split Plan (Frontend + PayLock Infra)

## Goal
Split the current codebase into two repositories:
- **Infra repo**: `paylock` (backend/API + on-chain infra support)
- **Frontend repo**: application layer UI

This document captures the planned updates for the current repo **before** implementation.

## Key Decisions
- Infra will be public and usable by any frontend.
- `/api/*` endpoints in infra should support CORS for cross-origin frontends.
- Final state is **two separate repos**, not a monorepo with two folders.

## Scope of Changes (Infra Repo)

### Code Changes
1. **Remove embedded frontend SPA**
   - Current SPA assets live in `cmd/paylock/web/`
   - Server embeds SPA and serves `/` via `go:embed` in `cmd/paylock/main.go`
   - Plan: remove static assets from infra repo and delete the embed/`GET /` handlers

2. **CORS for `/api/*`**
   - Current state: only `/stream/*` has CORS
   - Plan: enable CORS for `/api/*` to allow browser clients from independent frontend domains
   - Provide configuration for allowed origins (env-based) to keep it secure by default

### Documentation Changes
1. **README.md (Infra)**
   - Remove references to built-in SPA
   - Update architecture diagram/description to show external frontend
   - Clarify infra as standalone backend service

2. **API.md (Infra)**
   - Remove “built-in SPA” references
   - Reframe examples as “external frontend integration”

3. **Agent docs**
   - Update `CLAUDE.md` / `GEMINI.md` to reflect infra-only state

## Scope of Changes (Frontend Repo)
- Create new repo with its own README
- Document setup, env vars (e.g. `PAYLOCK_BASE_URL`), and integration steps
- Include any wallet integrations and UI-specific flows

## Non-Goals
- No refactors to business logic at this stage
- No new features beyond CORS and repo split cleanup

## Open Questions
- What should infra return for `/` (404 vs health/info page vs redirect)?
- CORS policy: allow all origins or restrict via env allowlist?
- Do we keep any minimal static docs page in infra?

## Proposed Order of Work
1. Remove embedded SPA and related routes
2. Add `/api/*` CORS support
3. Update infra docs (README, API.md, agent docs)
4. Create frontend repo and migrate UI assets

