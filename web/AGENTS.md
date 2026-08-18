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
  preparing/applied/screening/responded/interview/offer/accepted/rejected/withdrawn/expired.
- A view is recorded silently when a signed-in user opens a job — failure is swallowed and
  must not break the page.
- `MatchSummary.svelte` is the compact sidebar summary (overall % + verdict + top gap)
  linking to the full analysis in the `/tailor/[slug]` workspace — it never computes inline.
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
  in dev. At boot the start is deferred to `requestIdleCallback` (3s timeout) so gtag.js and
  the PostHog SDK stop competing with first paint; **calls made before the SDK resolves are
  queued in `$lib/analytics` and replayed in order**, which is what keeps the initial
  `afterNavigate` pageview and — load-bearing — the `/my/**` `stopSessionRecording()` from
  being dropped. Accept-in-the-banner still starts them immediately.
- **`hooks.server.ts` narrows the `Link` header** to the stylesheet and the two entry modules.
  SvelteKit's default preloads a route's whole module graph, which here is ~85 sub-1.5KB
  chunks — ~90 `modulepreload` fetches racing the render-blocking CSS. Removing them cut
  mobile LCP from 8.5s to 4.4s on `/` and 7.8s to 4.7s on a job page (Lighthouse, Slow 4G,
  same rig). Note `sequence()` semantics: the FIRST `preload` wins, later ones are ignored.
- The header's GitHub star count comes from **our own `/github-stars`**, not `api.github.com`.
  Called from the browser it spent the visitor's share of GitHub's 60/hour-per-IP cap and
  returned 403 from any shared address; `$lib/server/github` holds one process-wide hourly
  cache, shared with `/open`. `stars: null` is a valid answer — the badge drops the number,
  never errors. Unauthenticated even server-side, so a busy IP still degrades to null.
- The PWA service worker (`web/vite.config.ts`, `SvelteKitPWA`) precaches only the built app
  shell — no runtime-caching entry for `/api/*` or navigations, since every job listing,
  filter result and `/me/*` response is personalized or live. `workbox.navigateFallback` is
  explicitly `null`: left unset it defaults to `'/'`, and since this app is full SSR with no
  prerendered `/`, that would route every navigation through a handler bound to a URL that was
  never precached — on a shared device that's a session-personalized shell served to the next
  visitor. `registerType: 'autoUpdate'` reloads open tabs on a new deploy; `onNeedReload` in
  `+layout.svelte` gates that behind a confirmation so it can't silently drop an in-progress
  CV edit or application form.

## Worth knowing

- **Job-fit analysis**: the standalone `/match/[slug]/` page is gone — its
  `+page.server.ts` is only a 308 redirect to `/tailor/[slug]`, and the Tailor workspace
  is the sole surface for viewing and triggering the analysis. The analysis UI is
  `MatchAnalysisFull.svelte` (stepper + thinking panel + progressive sections over an
  `EventSource`), embedded in the workspace's `ArtifactPanel` and in `JobDrawer.svelte`.
  The pure SSE reducer `reduceMatchEvent` lives in `web/src/lib/matchAnalysis.ts`
  (unit-tested).
- **`ReferralBlock` imports its modal on the click that opens it.** The block is small; the
  `RequestReferralModal` behind it pulls in Dialog, FormField and the request flow, and it sat
  in the module graph of every job and company page while only the few visitors who actually
  ask for a referral open it. The block itself stays server-rendered — deferring it too would
  push the description down after hydration and trade the CLS the page currently has at zero.
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
- `easymde` (the `NoteEditor.svelte` markdown field for private job-tracking notes) carries a
  known ReDoS in its bundled `codemirror@5.x`, with no safe upgrade or override available —
  see `web/.snyk` for the accepted-risk reasoning and its revisit date.
