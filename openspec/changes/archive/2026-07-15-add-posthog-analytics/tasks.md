## 1. Dependency & config

- [x] 1.1 Add `posthog-js` to `web/package.json` and install
- [x] 1.2 Document `PUBLIC_POSTHOG_KEY` (+ optional `PUBLIC_POSTHOG_HOST`
      defaulting to `/ingest`) in the frontend env/config reference; confirm dev
      leaves the key unset

## 2. Analytics module (`src/lib/analytics.ts`)

- [x] 2.1 Write failing vitest for the pure helpers: `isPrivateRoute(path)` (true
      for `/my/*` and inbox, false for public), `track()` no-op safety when
      uninitialized, and `isFeatureEnabled(flag, fallback)` returning the fallback
      when PostHog is inert
- [x] 2.2 Implement `analytics.ts`: `track(event, props)`, `identifyUser(user)` /
      `resetIdentity()` (id only, no email), `syncReplayForRoute(path)`, and a
      generic `isFeatureEnabled(flag, fallback)` reader — all safe no-ops when
      PostHog is uninitialized; make tests green
- [x] 2.3 simplify pass on the module; re-run vitest green

## 3. Initialization (`hooks.client.ts`)

- [x] 3.1 Initialize PostHog env-gated by `PUBLIC_POSTHOG_KEY` with `api_host`
      `/ingest`, `ui_host` EU, `capture_pageview:false`,
      `person_profiles:'identified_only'`, `session_recording:{maskAllInputs:true}`;
      inert when key absent (mirror Sentry block)

## 4. Routing-dependent wiring (`+layout.svelte`)

- [x] 4.1 In `afterNavigate` (and on initial load): capture `$pageview`, call
      `identifyUser`/`resetIdentity` from `page.data.user`, and
      `syncReplayForRoute` to start/stop recording per route

## 5. Funnel events

- [x] 5.1 Fire `search` on `/jobs` when filters/query are applied (query + active
      facet count)
- [x] 5.2 Fire `job_view` on job card/page open (slug, source)
- [x] 5.3 Fire `job_apply`, `job_save`, `job_track` on the respective action
      buttons (slug, source; stage for track) — independent of auth state

## 6. Feature flag seam

- [x] 6.1 Ship the generic `isFeatureEnabled(flag, fallback)` reader (from 2.2)
      as the flag capability; leave wiring a concrete product default (e.g.
      `default-hide-nontech`, not present on origin/main) as a documented seam —
      no consumer added in this change

## 7. Ops documentation

- [x] 7.1 Document the `/ingest/` nginx reverse-proxy location block (events +
      static assets → `eu.i.posthog.com`) for `freehire-ops`, ordered before the
      SvelteKit catch-all

## 8. Verify

- [x] 8.1 Static verification green: `svelte-check` (0 errors), vitest (270
      pass), and prod `build` (SSR import-safe). Live smoke — events reaching
      `/ingest`, replay off on `/my/*` with a real key + nginx proxy — is a
      deploy-time step (mirrors the archived Sentry change), not run pre-merge.
