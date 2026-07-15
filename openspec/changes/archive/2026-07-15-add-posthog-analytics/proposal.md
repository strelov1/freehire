## Why

GA4 tells us pageviews but nothing about the product funnel — how many searchers
reach a job page, how many of those apply/save/track, or where users fight the
filter and swipe UIs. We have no way to record real sessions, and feature gating
(e.g. non-tech default) is hardcoded with no way to flip it without a deploy.
PostHog Cloud (EU) closes all three gaps with one client SDK.

## What Changes

- Add `posthog-js`, initialized in `hooks.client.ts`, env-gated by
  `PUBLIC_POSTHOG_KEY` — fully inert when the key is absent (mirrors the Sentry
  integration; no key in dev = silent).
- Route all PostHog traffic through a same-origin reverse proxy (`/ingest/`) so
  ad-blockers and CSP don't drop events; no external host is added to CSP.
- Capture SPA pageviews, `identify`/`reset` signed-in users by id only (no
  email — matches `sendDefaultPii:false`), all from `+layout.svelte`
  `afterNavigate`.
- Enable **session replay** with `maskAllInputs`, and disable recording entirely
  on private routes (`/my/*`, inbox) via a route-based toggle.
- Emit a small set of explicit funnel events (`search`, `job_view`, `job_apply`,
  `job_save`, `job_track`) through a thin `$lib/analytics.ts` wrapper; keep
  autocapture on for everything else.
- Expose a generic client-side **feature-flag reader** (`isFeatureEnabled`) as
  the flag capability. Wiring a concrete product default (e.g.
  `default-hide-nontech`, not on origin/main) is left as a seam; backend flag
  migration is out of scope.

## Capabilities

### New Capabilities
- `product-analytics`: client-side product analytics via PostHog — env-gated init
  behind a same-origin reverse proxy, SPA pageview capture, identity binding,
  privacy-scoped session replay, explicit funnel events, and a client feature
  flag.

### Modified Capabilities
<!-- No existing spec's requirements change. GA4 stays; error-tracking (Sentry)
     is untouched and complementary. -->

## Impact

- **Frontend (`web/`)**: `package.json` (+`posthog-js`), `hooks.client.ts`
  (init), `+layout.svelte` (pageview/identify/replay toggle), new
  `src/lib/analytics.ts`, and funnel-event call sites on `/jobs` search + job
  action buttons (apply/save/track).
- **Env**: new `PUBLIC_POSTHOG_KEY` (and optional `PUBLIC_POSTHOG_HOST`,
  defaulting to `/ingest`). Documented in config; absent in dev.
- **Ops (`freehire-ops`, host2 nginx)**: a `/ingest/` reverse-proxy location
  block to `eu.i.posthog.com` (+ static assets). Documented here, applied out of
  band — no change in this repo.
- **Not touched**: GA4 (kept for SEO/traffic), Sentry, backend, no cookie-consent
  banner (deferred gap, matches current GA posture).
