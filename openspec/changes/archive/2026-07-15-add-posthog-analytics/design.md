## Context

The SvelteKit SSR frontend (`web/`) already loads GA4 inline in `app.html`
(pageviews only, skipped on localhost) and Sentry via `hooks.client.ts`
(env-gated by `PUBLIC_SENTRY_DSN`, errors-only, `sendDefaultPii:false`). The
signed-in user is resolved server-side in `+layout.server.ts` and exposed as
`page.data.user` everywhere. There is no cookie-consent banner today. CSP is set
at the nginx layer on host2 (the reason `gtag` is partially ad-block/CSP dropped).

PostHog Cloud EU is chosen for data residency (the site handles CV, inbox, and
tracking PII).

## Goals / Non-Goals

**Goals:**
- Product funnel analytics (`search → job_view → apply/save/track`).
- Session replay on public routes to debug filter/swipe UX.
- One client feature flag as a demonstrator (non-tech default).
- Zero external hosts in CSP; resilient to ad-blockers.
- Inert-by-default, mirroring the Sentry env-gating pattern.

**Non-Goals:**
- Removing GA4 (kept for SEO/traffic; the two do not merge).
- Server-side flag evaluation or migrating the backend `beta_tester` gate to
  PostHog (noted as a seam; needs a server SDK + SSR flag reads).
- A cookie-consent banner (deferred gap; matches current GA posture).
- Formal A/B experiments (let events accumulate first).

## Decisions

- **SDK via npm (`posthog-js`), init in `hooks.client.ts`** — mirrors Sentry;
  bundled into app JS so nothing external loads. Env-gated by
  `PUBLIC_POSTHOG_KEY`; `capture_pageview:false` (SPA nav captured manually),
  `person_profiles:'identified_only'` (no anon profiles → saves quota),
  `session_recording:{ maskAllInputs:true }`.
- **Reverse proxy `/ingest/`** (nginx, `freehire-ops`) → `eu.i.posthog.com`
  (+ `/ingest/static/` → `eu-assets.i.posthog.com`). `api_host:'/ingest'`,
  `ui_host:'https://eu.posthog.com'`. Location block must precede the SvelteKit
  catch-all. Documented here; applied out of band (no change in this repo). Watch
  for `proxy_buffer_size` 502s on large replay payloads (known host2 quirk).
- **Routing-dependent logic centralized in `+layout.svelte` `afterNavigate`**:
  capture `$pageview`; `identify(user.id)` / `reset()` on auth transitions
  (id only, no email); toggle replay — `stopSessionRecording()` on `/my/*` and
  inbox, `startSessionRecording()` elsewhere.
- **`src/lib/analytics.ts` wrapper** exposing `track(event, props)` and flag
  helpers; every call safe no-op when PostHog is uninitialized. Funnel events
  fire on the UI action regardless of auth (unlike the authed-only interaction
  API). Autocapture stays on for the long tail.
- **Feature flag** read client-side via the analytics module to control
  `default-hide-nontech`; falls back to the existing hardcoded default when the
  flag is unavailable.

## Risks / Trade-offs

- **No consent banner + EU replay** — recording DOM on an EU property without
  consent is a known GDPR gap. Mitigated by `maskAllInputs` + private-route
  exclusion; accepted for now to match GA posture, flagged for a later pass.
- **Replay toggle races the first paint** — `afterNavigate` fires after mount, so
  a private route entered via full reload could record briefly. Mitigation:
  also evaluate the route on initial load, not only on navigation.
- **GA4 + PostHog overlap** on pageviews — accepted; they serve different
  consumers (SEO vs product). No attempt to reconcile counts.
- **Quota** — free tier is 1M events + 5k replays/mo; `identified_only` profiles
  and public-only replay keep usage down. Revisit if we approach the ceiling.
