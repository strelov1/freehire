# SPA conventions

## Scope
Svelte SPA under `web/` consuming the freehire API (same-origin; Vite proxy forwards `/api` in dev).

## Always true
- Auth is cookie-based via `HttpOnly; SameSite=Lax` — never a Bearer header or `localStorage`. The SPA cannot read the token (XSS-safe); the browser attaches it automatically.
- Same-origin in dev: the Vite proxy (`web/vite.config.ts`) forwards `/api` to the backend. `SameSite=Lax` + same-origin is the CSRF defense (no CSRF token needed).
- OAuth buttons render from `GET /api/v1/auth/oauth/providers` (lists enabled providers). OAuth callbacks 302 back to the SPA; failures 302 with `?auth_error=oauth`, never JSON.
- `stage` in job tracking mirrors the backend controlled vocabulary (`internal/userjob/stages.go`): applied/screening/responded/interview/offer/accepted/rejected/withdrawn.
- The SPA records a view silently when a signed-in user opens a job — failure is swallowed and must not break the page.
- `JobFitAnalysis.svelte` is a compact summary (overall % + verdict + top gap) linking to the full `/jobs/[slug]/fit/` page; it never computes inline.
- `ResumeStructuredView.svelte` is read-only — the structured resume is display-only, never editable.
- Sentry is gated on `PUBLIC_SENTRY_DSN` (+ `PUBLIC_SENTRY_ENVIRONMENT`); source map upload only when `SENTRY_AUTH_TOKEN`/`SENTRY_ORG`/`SENTRY_PROJECT` are set (build succeeds without them).

## How it works
The SPA is built with SvelteKit and consumes the API at `/api/v1/*`. Auth is handled via httpOnly cookies, transported transparently by the browser (no client-side token management). The Vite proxy forwards `/api` to the backend in dev; in production the SPA and API are same-origin.

**Auth surface:** `register`/`login`/`logout`/`me` endpoints set and clear the session cookie. OAuth sign-in buttons are driven by the `/api/v1/auth/oauth/providers` list. The callback flow redirects back to the SPA on success or with `?auth_error=oauth` on failure.

**Job-fit analysis page** (`web/src/routes/jobs/[slug]/fit/`): `+page.server.ts` SSRs a fresh cached analysis for an instant paint; otherwise the page opens an `EventSource` and renders a stepper + a thinking panel + progressive sections. The pure SSE reducer `reduceFitEvent` lives in `web/src/lib/jobFit.ts` (unit-tested). `JobFitAnalysis.svelte` (`web/src/lib/components/`) is the compact sidebar summary that links to the full page — it never computes inline.

**Structured resume** (`ResumeStructuredView.svelte`): rendered read-only in the profile's readiness tab. The backend serves it from `GET /api/v1/me/resume` (the `structured` field, null when absent/stale/unconfigured).

**Filters:** the companies FilterModal uses `COMPANY_FACETS` from `web/src/lib/facets.ts`, including a "Remote hiring" pill that reuses the shared `REGION` vocabulary for the `remote_regions` overlap facet.

## Limitations
- Announcing shipped work via the `/blog` changelog (the `write-changelog` skill) is an agent-level concern documented in the root `AGENTS.md` — out of scope for the frontend module.
- No CSP `connect-src` restriction is set — a comment in `web/svelte.config.js` records the ingest host for any future `connect-src` if one is added.
- OAuth identity unlinking/management UI is not implemented (backend seam mirrored on the frontend).
