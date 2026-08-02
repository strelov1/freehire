# SPA conventions

SvelteKit app under `web/`, consuming the freehire API at `/api/v1/*`. Same-origin in
production; in dev the Vite proxy (`web/vite.config.ts`) forwards `/api` to the backend.

## Always true

- Auth is cookie-based via `HttpOnly; SameSite=Lax` — never a Bearer header or
  `localStorage`. The SPA cannot read the token (XSS-safe); the browser attaches it
  automatically. `SameSite=Lax` + same-origin **is** the CSRF defence — no CSRF token.
- OAuth buttons render from `GET /api/v1/auth/oauth/providers`. Callbacks 302 back to the
  SPA; failures 302 with `?auth_error=oauth`, never JSON.
- `stage` in job tracking mirrors the backend vocabulary (`internal/userjob/stages.go`):
  applied/screening/responded/interview/offer/accepted/rejected/withdrawn/expired.
- A view is recorded silently when a signed-in user opens a job — failure is swallowed and
  must not break the page.
- `MatchSummary.svelte` is the compact sidebar summary (overall % + verdict + top gap)
  linking to the full `/match/[slug]/` page — it never computes inline.
- The profile headshot (`HeadshotField.svelte`, profile Settings tab) is one image per member,
  reused by every CV. `GET /api/v1/me/photo` is metadata — the bytes are the `/image`
  sub-resource, and the URL carries `?v=<uploaded_at>` because it is otherwise stable. The
  control renders nothing when `enabled` is false (object storage unconfigured).
- `ResumeStructuredView.svelte` is read-only; the backend serves the structured CV from
  `GET /api/v1/me/resume` (`structured` field, null when absent/stale/unconfigured).
- Sentry is gated on `PUBLIC_SENTRY_DSN` (+ `PUBLIC_SENTRY_ENVIRONMENT`); source-map upload
  only when `SENTRY_AUTH_TOKEN`/`SENTRY_ORG`/`SENTRY_PROJECT` are set (build succeeds
  without them).
- PostHog is gated on `PUBLIC_POSTHOG_KEY` (inert without it — no init, no events);
  ingestion goes through the same-origin `/ingest` reverse proxy (nginx → `eu.i.posthog.com`),
  overridable via `PUBLIC_POSTHOG_HOST`. Injected by `freehire-ops`, never committed, unset
  in dev.

## Worth knowing

- **Job-fit analysis** (`web/src/routes/match/[slug]/`): `+page.server.ts` SSRs a fresh
  cached analysis for an instant paint; otherwise the page opens an `EventSource` and
  renders a stepper + thinking panel + progressive sections. The pure SSE reducer
  `reduceMatchEvent` lives in `web/src/lib/matchAnalysis.ts` (unit-tested).
- **Filters**: the companies FilterModal uses `COMPANY_FACETS` from `web/src/lib/facets.ts`,
  including a "Remote hiring" pill that reuses the shared `REGION` vocabulary for the
  `remote_regions` overlap facet.

## Limitations

- No CSP `connect-src` restriction is set — a comment in `web/svelte.config.js` records the
  ingest host for any future `connect-src`.
- OAuth identity unlinking/management UI is not implemented (backend seam mirrored on the
  frontend).
