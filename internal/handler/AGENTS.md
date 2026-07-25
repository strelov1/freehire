# internal/handler — HTTP Handlers

Fiber HTTP handlers: feature handler structs, route registration, auth surface, user job endpoints, error rendering.

## Architecture

- `API` (`handler.go`) holds only the cross-cutting dependencies: the DB pool, the sqlc
  queries, and the token issuer the auth middleware is built from. `Register` builds the
  shared services, constructs every feature handler, and calls each feature's `register`
  in an order that keeps literal routes before param routes (e.g. `/jobs/search` before
  `/jobs/:slug`, `/threads/count` before `/threads/:id`, the static `/me/tracking/*`
  before `/me/tracking/:slug`).
- Each feature area owns a struct with its own dependencies, a constructor
  (`newXHandlers`), and a `register(api fiber.Router, mw middleware)` method that mounts
  its routes. Handlers are thin — auth primitives, user job operations, API key
  management, errors live in separate files. The features and their files:
  - `authHandlers` — auth.go (register/login/logout/me), oauth.go, api_keys.go,
    extension_connect.go
  - `jobsHandlers` — jobs.go, copies.go, jobs_moderation.go (moderator-authored writes)
  - `searchHandlers` — search.go, agent_search.go, similar.go, facets.go
  - `companiesHandlers` — companies.go
  - `sitemapHandlers` — sitemap.go; `statsHandlers` — stats.go, stats_facets.go,
    status.go, insights.go
  - `trackingHandlers` — user_jobs.go, me_tracking.go, me_reminders.go, swipe.go
  - `voteHandlers` — votes.go; `communityHandlers` — community.go
  - `submissionHandlers` — submissions.go; `contributionHandlers` — contributions.go;
    `reportHandlers` — reports.go; `referralHandlers` — referrals.go
  - `savedSearchHandlers` — me_searches.go, boards.go; `subscriptionHandlers` —
    me_subscriptions.go; `profileHandlers` — me_profile.go; `creditsHandlers` —
    me_credits.go, me_credits_history.go
  - `matchHandlers` — match_analysis.go, match_analysis_stream.go, me_analyses.go,
    job_match.go, hardconstraint_inputs.go
  - `cvHandlers` — cv.go, cv_tailor.go (holds a `*matchHandlers` for the blocker/credits
    helpers tailoring reuses)
  - `resumeHandlers` — resume.go, resume_verdict.go, ats_report.go, recommendations.go,
    market_coverage.go
  - `inboxHandlers` — inbox.go, inbox_linking.go, gmail.go, mailbox.go;
    `telegramHandlers` — telegram.go
- The `middleware` bundle (`handler.go`) carries the shared auth gates features mount
  behind: `optional` (attach caller, never reject), `key` (cookie or API key), `cookie`
  (cookie-only), `moderator` (role gate, stacked after `key`/`cookie`).
- Services shared across features (résumé store, profile, credits, CV store/renderer,
  match analyzer, contribution, moderation) are built once in `Register` and passed to
  each constructor; single-feature services are built inside the feature's constructor.
- Tests construct the feature struct directly with fakes/stubs (e.g.
  `&trackingHandlers{tracking: ...}`) and mount routes on a bare `fiber.App`.
- Central `handler.RenderError` (wired in `cmd/server` via `fiber.Config{ErrorHandler: handler.RenderError}`) renders JSON envelope: `*fiber.Error`→its code, `pgx.ErrNoRows`→404, FK-violation (SQLSTATE 23503)→404, everything else→500.
- Handlers signal failure by returning an error — `fiber.NewError(status, msg)` for specific codes, bare error (e.g. `pgx.ErrNoRows`) for common cases. Don't hand-roll per-handler error JSON; don't re-map `ErrNoRows` in read handlers (just `return err`).

## Auth Handlers (`auth.go`)

- `register`/`login` set JWT cookie + return `{"data": user}`. `logout` clears it. `me` is guarded by `RequireAuthOrKey` (cookie or API key).
- Rate-limited credential endpoints (10/min, keyed on client IP).

## User Job Handlers (`user_jobs.go`)

- `view`/`apply`/`save`/`track` interaction endpoints. Addressed by job's public `:slug` (resolved to internal id before write). All writes are idempotent upserts behind `RequireAuthOrKey`.
- Return `{"data": interaction}` with `user_id` omitted; public job reads stay unauthenticated.

## Error Convention

- Genuinely domain-specific status choices (e.g. `Me` returning 401 for a gone user token) stay in the handler.
- Recovered panic is **not** double-reported (recover middleware marks it via `handler.LocalPanicReported`).
- Sentry reports only fall-through unexpected 500s — routine errors never reported.
