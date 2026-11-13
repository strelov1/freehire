# SPA conventions

SvelteKit app under `web/`, consuming the freehire API at `/api/v1/*`. Same-origin in
production; in dev the Vite proxy (`web/vite.config.ts`) forwards `/api` to the backend.

## Always true

- Auth is cookie-based via `HttpOnly; SameSite=Lax` — never a Bearer header or
  `localStorage`. The SPA cannot read the token (XSS-safe); the browser attaches it
  automatically. `SameSite=Lax` + same-origin **is** the CSRF defence — no CSRF token.
  Lax governs only whether an EXISTING cookie rides along; a NEW session cookie a
  login response issues is bound to request origin server-side instead
  (`requireSameOrigin`, `internal/api/handler/request_origin.go`): a present Origin
  or Referer must name a served origin, and `Sec-Fetch-Site: cross-site` refuses
  even when both were stripped; only the genuinely headerless shape (curl, the CLI)
  passes with no metadata. Refusals are 403 with code `cross_site_refused`;
  classify with `isCrossSiteRefusal` (`#lib/api.js`), which reads the code, not the
  bare status.
- OAuth buttons render from `GET /api/v1/auth/oauth/providers`. Callbacks 302 back to the
  SPA; failures 302 with `?auth_error=oauth`, never JSON.
- Four actions need a *second* proof on top of the session — creating and revoking an API
  key, changing the password, deleting the account — and answer `428 recent_auth_required`
  without one. `ConfirmIdentity.svelte` is the only surface that asks for it: it decides
  whether the account confirms by password or by provider, `prove()` produces the proof (and
  refuses to send a blank one), and `handleRefusal()` words the `401`/`428` so no caller
  keeps its own copy. Inside a `Dialog`, pass `active={open}` — `Dialog` renders its children
  unconditionally, so an always-active instance fetches providers on pages whose dialog is
  never opened. The exception is the password-change form: the current password already *is*
  the confirmation, so a `428` there means the session cannot bind a proof at all and the
  only remedy is signing in again.
- A pending action survives the provider round trip in `sessionStorage` (`recentAuth.ts`,
  `ReauthDraft`), because confirming leaves the site entirely. Secrets and anti-mistake
  barriers never go in it: the typed key name is restored, the typed address confirming an
  account deletion deliberately is not.
- `stage` in job tracking mirrors the backend vocabulary (`internal/application/userjob/stages.go`):
  preparing/applied/screening/responded/interview/offer/accepted/rejected/withdrawn/expired.
- A view is recorded silently when a signed-in user opens a job — failure is swallowed and
  must not break the page.
- `MatchSummary.svelte` is the compact sidebar summary (overall % + verdict + top gap)
  linking to the full analysis in the `/tailor/[slug]` workspace — it never computes inline.
- The profile headshot (`HeadshotField.svelte`, in `ProfileForm` on the profile's main tab) is one image per member,
  reused by every CV. `GET /api/v1/me/photo` is metadata — the bytes are the `/image`
  sub-resource, and the URL carries `?v=<uploaded_at>` because it is otherwise stable. The
  control renders nothing when `enabled` is false (object storage unconfigured).
- The profile tabs read the structured CV through one store, `src/lib/resume.svelte.ts`, from
  `GET /api/v1/me/resume` (`structured` field: banked experience and the candidate-owned
  contacts composed over the extract, so a stale extract does not empty it; null only when nothing worth serving is left).
- Environment variables reach the app only through `src/env.ts`. SvelteKit 3 exposes
  nothing undeclared — not even through the deprecated `$env/*` modules — so a variable
  missing there is `undefined` at runtime however the host sets it, and nothing reports it.
  Declare it there and read it from `$app/env/public` / `$app/env/private`.
- The public origin (`page.url.origin`, behind every canonical URL) is baked in at BUILD
  time from `ORIGIN` — `paths.origin` in `vite.config.ts`. adapter-node no longer reads
  `ORIGIN` at runtime, so setting it only on the running server does nothing.
- Error telemetry is gated on `PUBLIC_OTEL_LOGS_ENDPOINT` (browser) and the private
  `OTEL_EXPORTER_OTLP_*` endpoint (SSR); without them nothing is exported, and SSR only
  logs unexpected errors to its journal. See
  [internal/platform/observability/AGENTS.md](../internal/platform/observability/AGENTS.md).
- PostHog is gated on `PUBLIC_POSTHOG_KEY` (inert without it — no init, no events);
  ingestion goes through the same-origin `/ingest` reverse proxy (nginx → `eu.i.posthog.com`),
  overridable via `PUBLIC_POSTHOG_HOST`. Injected by `freehire-ops`, never committed, unset
  in dev. At boot the start is deferred to `requestIdleCallback` (3s timeout) so gtag.js and
  the PostHog SDK stop competing with first paint; **calls made before the SDK resolves are
  queued in `#lib/analytics.js` and replayed in order**, which is what keeps the initial
  `afterNavigate` pageview and — load-bearing — the `/my/**` `stopSessionRecording()` from
  being dropped. Accept-in-the-banner still starts them immediately.
- **`hooks.server.ts` narrows the `Link` header** to the stylesheet and the two entry modules.
  SvelteKit's default preloads a route's whole module graph, which here is ~85 sub-1.5KB
  chunks — ~90 `modulepreload` fetches racing the render-blocking CSS. Removing them cut
  mobile LCP from 8.5s to 4.4s on `/` and 7.8s to 4.7s on a job page (Lighthouse, Slow 4G,
  same rig). Note `sequence()` semantics: the FIRST `preload` wins, later ones are ignored.
- The header's GitHub star count comes from **our own `/github-stars`**, not `api.github.com`.
  Called from the browser it spent the visitor's share of GitHub's 60/hour-per-IP cap and
  returned 403 from any shared address; `#lib/server/github.js` holds one process-wide hourly
  cache, shared with `/open`. `stars: null` is a valid answer — the badge drops the number,
  never errors. Unauthenticated even server-side, so a busy IP still degrades to null.
- **There is no PWA any more** (removed 2026-10-18). It bought an icon and a window without an
  address bar: nothing was cached for offline use (every listing is live or personal), the
  hashed assets already carry a year-long immutable cache, and there is no web push. What it
  cost was a stale precached shell serving deleted chunks as the 500 page, and from 2026-09-08
  a precache whose every URL 404ed. `web/static/sw.js` is a kill switch for browsers that still
  have the old worker registered — it clears their caches and unregisters itself; nothing
  registers it otherwise. Do not bring a service worker back without a reason it serves.
- The boundaries between the app's parts are checked by easement against
  [easement.yml](easement.yml): a component never imports a route or `lib/server`,
  `src/lib` never imports a component, and only routes, hooks and tests reach `lib/server`.
  Any dependency between two parts that the file does not list fails, so a new one is
  written there with its reason, not worked around. Run `easement check` in `web/` after
  changing imports (`mise install` provides it and its indexer); `easement why <a> <b>`
  shows the imports behind an edge. There is no baseline of known violations, and
  `easement baseline` is not the way to get a change through.

## Worth knowing

- **API wire types come from the contract**: `src/lib/generated/api.ts` is generated
  from `openapi/api.openapi.json` by `make api-contract` (CI fails when it is stale), so
  a field the server renames breaks this build, not production. A response type in
  `api-types.ts` or `wire-types/` for a route in the contract is an alias
  (`export type EmailPrefs = components['schemas']['EmailPrefs']`), and a request body
  is checked with `satisfies components['schemas'][...]`. Hand-written shapes remain
  for routes that stay on Fiber (streams, uploads) and for client-side shapes the UI
  maps from; move the rest over when you touch them.
- **The API reference is generated from the contract**: `pnpm run gen:api-docs` renders
  the `/docs/api` and `/docs/api/internal` OpenAPI documents (`src/lib/docs/generated/`)
  and `docs/API*.md` from `openapi/api.openapi.json`, plus the filter table from
  `src/lib/docs/filters.ts`. Endpoint text is written in `typespec/` (see
  `internal/api/AGENTS.md`). `src/lib/docs/api-supplement.openapi.json` describes only the
  routes still outside the contract (auth, streams, PDF and photo reads); when one moves
  into the contract the generator fails until its supplement entry is deleted.
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
- **Mentorship ships behind `users.beta_tester`, WHOLE.** Every route it owns — the public
  directory, a mentor's page, and all four cabinet tabs — answers **404** to anybody
  without the flag, and `MentorBlock` renders nothing. The predicate is
  `#lib/server/mentorshipGate.js`. **Ending the beta is three edits**, not one: that file,
  `inBeta` in `MentorBlock.svelte`, and `betaOnly` on `/my/mentorship` in `accountNav.ts`
  — a door, an offer of the door, and a menu entry, deliberately separate. Grep for
  `beta_tester` and `betaOnly`; those three are all of it. Gating only the
  cabinet was considered and rejected: it would leave a visitor able to BOOK from a public
  profile and then unable to find the session again or cancel it, and the hour a mentor is
  holding is real. 404 rather than 403 — while the feature is unreleased the honest answer
  is that the page is not there, and a 403 advertises it to every crawler that finds the URL.
- **Mentorship** (`/mentors`, `/mentors/[slug]`, `/my/mentorship`): the arithmetic lives in
  `web/src/lib/mentorship.ts`, the same split `matchAnalysis.ts` holds — pure, `$app`-free,
  unit-tested. **Nothing in the slot path constructs a `Date`.** A slot arrives with
  `local_start` already resolved in the viewer's zone, so parsing it re-interprets it against
  the browser's: Tokyo's 16th at 01:00 is still the 15th in UTC, and the slot files under the
  day before the one offered. The month grid is integer arithmetic for the same reason. A
  BOOKING is the opposite case — it carries only instants, so the zone is applied here. Every
  slot renders its `utc_offset`, because on the autumn transition two slots share a wall clock
  and differ only there. The profile page is server-rendered and its SLOTS are not: the server
  cannot know the viewer's zone, and rendering them in UTC then swapping after hydration shows
  one set of hours and silently replaces it. Booker state is `?month`/`?date`/`?slot`, held in
  the component and only MIRRORED to the address bar — `page.url` lags a shallow
  `replaceState` (see `urlSynced.svelte.ts`), so deriving from it moves the URL and not the
  screen. That URL is also what survives the sign-in bounce, which is why the sign-in link is
  built from `location.search` rather than `promptSignIn()`. With no `?month` the calendar opens on the first
  month that has an open hour: its first read searches 62 days forward from today's UTC
  boundary (`openingSlotWindow`, the endpoint's maximum window, shared for caching). Calendar-month boundaries can
  miss valid slots: 30 days from late January can reach March.
- **`MentorBlock` asks from the browser, not from `load`.** It answers "does this company have
  a mentor?" out of the DIRECTORY narrowed to that company — never a second endpoint, which
  would be a second copy of the publication predicate. It runs client-side because the job page
  is the busiest surface here and roughly three quarters of this host's traffic is crawlers: a
  server-side call would spend an API request on every bot fetch to answer a question no bot
  acts on. It renders nothing when there is no mentor, so callers need no gate of their own.
- **`color-scheme` is declared in `app.css`**, light by default and dark under `.dark`. The
  menu a native `<select>` opens is drawn by the OS, and without this it follows the OS
  setting rather than the site's — a visitor whose machine is dark got a black dropdown over a
  white page. There is no way to style a native menu; `color-scheme` is the whole API, and it
  also covers scrollbars, form-control focus rings and date pickers.
- **Filters**: the companies FilterModal uses `COMPANY_FACETS` from `web/src/lib/facets.ts`,
  including a "Remote hiring" pill that reuses the shared `REGION` vocabulary for the
  `remote_regions` overlap facet.
- **`AssistantChat` has three hosts**: `/my/assistant`, `/tailor/[slug]`, and the profile
  page's Experience tab (`ExperienceAssistantPanel.svelte`). It needs a flex parent with a
  bounded height. A plain chat with nothing to resume opens blank and creates its session
  with the first message (`deferredNew`); every other preset, a support chat and a "New chat"
  click still create one on mount — so never mount those before the user asks for one, or
  every visitor mints an empty conversation. **`kickoff` is read once**, into a
  deliberately non-reactive `arrival`, and spent after the first send: setting the prop on a
  live chat does nothing, so aiming it at a new subject means remounting it (the Experience
  panel keys on a launch token). Unmounting cancels a streaming turn, which is why closing
  that panel hides it instead. The opening text for the `profile` interview comes from
  `profileKickoff` in `#lib/assistant/presets` — shared with the `?preset=profile&atoms=…`
  URL entry so the two cannot drift. That message names achievements by id and asks the
  agent to read them first; it deliberately does not name the tool, because it is recorded
  as the candidate's own message.
- **The Experience panel is docked to the VIEWPORT, not laid out in the tab.** It is owned
  by `ExperienceBankView` but positioned `fixed`, and `my/+layout.svelte` yields to it
  through `dockOffset()` in `#lib/assistantDock.svelte.ts` — a module store, because the
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
- **Bundle budgets**: `pnpm run size` (the `Bundle size budgets` step of the `web` CI job, after `Build`) holds the brotli size of the client entry and eight heavy routes to the limits in `web/.size-limit.mjs`. To raise one on purpose, edit its limit in that file in the same PR and state why in the PR body.

## Limitations

- No CSP `connect-src` restriction is set — a comment in `web/vite.config.ts` records the
  ingest host for any future `connect-src`.
- OAuth identity unlinking/management UI is not implemented (backend seam mirrored on the
  frontend).
- `easymde` (the `NoteEditor.svelte` markdown field for private job-tracking notes) carries a
  known ReDoS in its bundled `codemirror@5.x`, with no safe upgrade or override available —
  see `web/.snyk` for the accepted-risk reasoning and its revisit date.
