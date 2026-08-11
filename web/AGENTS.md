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
- **`AssistantChat` has three hosts**: `/my/assistant`, `/tailor/[slug]`, and the profile
  page's Experience tab (`ExperienceAssistantPanel.svelte`). It needs a flex parent with a
  bounded height, and it boots a session on mount — so never mount it before the user asks
  for one, or every visitor mints an empty conversation. **`kickoff` is read once**, into a
  deliberately non-reactive `arrival`, and spent after the first send: setting the prop on a
  live chat does nothing, so aiming it at a new subject means remounting it (the Experience
  panel keys on a launch token). Unmounting cancels a streaming turn, which is why closing
  that panel hides it instead. The opening text for the `profile` interview comes from
  `profileKickoff` in `$lib/assistant/presets` — shared with the `?preset=profile&atoms=…`
  URL entry so the two cannot drift. That message names achievements by id and asks the
  agent to read them first; it deliberately does not name the tool, because it is recorded
  as the candidate's own message.
- **The Experience panel is docked to the VIEWPORT, not laid out in the tab.** It is owned
  by `ExperienceBankView` but positioned `fixed`, and `my/+layout.svelte` yields to it
  through `dockOffset()` in `$lib/assistantDock.svelte.ts` — a module store, because the
  panel opens inside a page and the offset is applied by the layout across a route
  boundary. It was a column inside the content area and spent the bank's width; the shell
  now steps aside instead. The layout also collapses the section nav to its icon rail while
  the dock is open, which is what keeps the bank at least as wide as it was — that override
  is never persisted, and an explicit toggle cancels the restore. Docking starts at
  `DOCKED_QUERY` (85rem); below it the panel covers the bank as a modal overlay and claims
  no offset. Both numbers are derived in `assistantDock.svelte.ts` — change one and redo
  the arithmetic.
- **The `TopBar` is `sticky top-0 z-40` and `h-14`.** Anything else that pins to the top of
  the page needs `top-14` and a z-index below 40, or it pins *behind* the header and is
  invisible exactly when it is needed.

## Limitations

- No CSP `connect-src` restriction is set — a comment in `web/svelte.config.js` records the
  ingest host for any future `connect-src`.
- OAuth identity unlinking/management UI is not implemented (backend seam mirrored on the
  frontend).
