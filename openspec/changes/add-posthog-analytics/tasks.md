## 1. Dependency & config

- [ ] 1.1 Add `posthog-js` to `web/package.json` and install
- [ ] 1.2 Document `PUBLIC_POSTHOG_KEY` (+ optional `PUBLIC_POSTHOG_HOST`
      defaulting to `/ingest`) in the frontend env/config reference; confirm dev
      leaves the key unset

## 2. Analytics module (`src/lib/analytics.ts`)

- [ ] 2.1 Write failing vitest for the pure helpers: `isPrivateRoute(path)` (true
      for `/my/*` and inbox, false for public), `track()` no-op safety when
      uninitialized, and the non-tech flag fallback resolver
- [ ] 2.2 Implement `analytics.ts`: `track(event, props)`, `identifyUser(user)` /
      `resetIdentity()` (id only, no email), `syncReplayForRoute(path)`, and
      `nonTechDefaultHidden()` flag reader with hardcoded fallback — all safe
      no-ops when PostHog is uninitialized; make tests green
- [ ] 2.3 simplify pass on the module; re-run vitest green

## 3. Initialization (`hooks.client.ts`)

- [ ] 3.1 Initialize PostHog env-gated by `PUBLIC_POSTHOG_KEY` with `api_host`
      `/ingest`, `ui_host` EU, `capture_pageview:false`,
      `person_profiles:'identified_only'`, `session_recording:{maskAllInputs:true}`;
      inert when key absent (mirror Sentry block)

## 4. Routing-dependent wiring (`+layout.svelte`)

- [ ] 4.1 In `afterNavigate` (and on initial load): capture `$pageview`, call
      `identifyUser`/`resetIdentity` from `page.data.user`, and
      `syncReplayForRoute` to start/stop recording per route

## 5. Funnel events

- [ ] 5.1 Fire `search` on `/jobs` when filters/query are applied (query + active
      facet count)
- [ ] 5.2 Fire `job_view` on job card/page open (slug, source)
- [ ] 5.3 Fire `job_apply`, `job_save`, `job_track` on the respective action
      buttons (slug, source; stage for track) — independent of auth state

## 6. Feature flag demonstrator

- [ ] 6.1 Gate the `default-hide-nontech` default via `nonTechDefaultHidden()`,
      preserving current behavior when the flag is unavailable

## 7. Ops documentation

- [ ] 7.1 Document the `/ingest/` nginx reverse-proxy location block (events +
      static assets → `eu.i.posthog.com`) for `freehire-ops`, ordered before the
      SvelteKit catch-all

## 8. Verify

- [ ] 8.1 Run `svelte-check` + vitest; visual-verify a public route emits events
      to `/ingest` and a `/my/*` route does not record (dev with a test key)
