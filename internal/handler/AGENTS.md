# internal/handler — HTTP Handlers

Fiber HTTP handlers: API struct, route registration, auth surface, user job endpoints, error rendering.

## Architecture

- `API` struct + `Register` wires routes. Handlers are thin — auth primitives, user job operations, API key management, errors live in separate files.
- Central `handler.RenderError` (wired in `cmd/server` via `fiber.Config{ErrorHandler: handler.RenderError}`) renders JSON envelope: `*fiber.Error`→its code, `pgx.ErrNoRows`→404, FK-violation (SQLSTATE 23503)→404, everything else→500.
- Handlers signal failure by returning an error — `fiber.NewError(status, msg)` for specific codes, bare error (e.g. `pgx.ErrNoRows`) for common cases. Don't hand-roll per-handler error JSON; don't re-map `ErrNoRows` in read handlers (just `return err`).

## Auth Handlers (`auth.go`)

- `register`/`login` set JWT cookie + return `{"data": user}`. `logout` clears it. `me` is guarded by `RequireAuth` middleware.
- Rate-limited credential endpoints (10/min, keyed on client IP).

## User Job Handlers (`user_jobs.go`)

- `view`/`apply`/`save`/`track` interaction endpoints. Addressed by job's public `:slug` (resolved to internal id before write). All writes are idempotent upserts behind `RequireAuthOrKey`.
- Return `{"data": interaction}` with `user_id` omitted; public job reads stay unauthenticated.

## Error Convention

- Genuinely domain-specific status choices (e.g. `Me` returning 401 for a gone user token) stay in the handler.
- Recovered panic is **not** double-reported (recover middleware marks it via `handler.LocalPanicReported`).
- Sentry reports only fall-through unexpected 500s — routine errors never reported.
