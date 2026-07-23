## Context

Two non-essential trackers ship today with no consent gate:

- **Google Analytics** — an inline `<script>` in `web/src/app.html` that fires on
  every non-localhost load. It is explicitly allow-listed in the CSP
  (`web/svelte.config.js`: the gtag bootstrap's SHA-256 hash plus the
  `https://www.googletagmanager.com` host), so it is live and sets `_ga` cookies.
- **PostHog** — initialized in `web/src/hooks.client.ts` via the guarded wrapper
  in `web/src/lib/analytics.ts`, with session recording started per-route in
  `web/src/routes/+layout.svelte`.

There is no consent banner anywhere in `web/src`. The privacy policy
(`web/src/routes/privacy/+page.svelte`) claims cookies load only "if you opt in"
and that we keep "no tracking profiles" — both false today.

Constraints: nginx on bare metal, no Cloudflare, no `cf-ipcountry`, no server-side
geo signal. The project is frontend-SSR (SvelteKit) fronting a Go API. House style
(analytics.ts) is a guarded, env-gated wrapper whose call sites are safe no-ops.

## Goals / Non-Goals

**Goals:**

- Block GA and PostHog for consent-required (EU/EEA/UK) visitors until they accept.
- Show the banner only where legally required; leave non-region visitors untouched.
- No new infrastructure, no IP handling, no ops/nginx change.
- Consent is withdrawable as easily as it is given.
- Reconcile the privacy policy with what the code actually does.

**Non-Goals:**

- Per-category consent toggles (only two trackers, no advertising → binary
  Accept/Reject is sufficient).
- Server-side / IP-based geolocation, GeoIP databases, a nginx country header.
- A US-style "Do Not Sell/Share" opt-out flow (tracked separately if ever needed).
- Any backend, API, or database change.

## Decisions

### D1 — Region signal from browser timezone, client-side

Use `Intl.DateTimeFormat().resolvedOptions().timeZone`; classify `Europe/*` and
the EEA Atlantic zones (e.g. `Atlantic/Canary`, `Atlantic/Madeira`,
`Atlantic/Reykjavik`) as consent-required. Unresolvable/unknown → consent-required.

- *Why:* zero infrastructure, no IP processing (privacy-friendlier), available at
  boot. Errs conservative — a few non-EU visitors (Ukraine, Turkey) may see the
  banner, which is harmless, while EU visitors are reliably caught.
- *Alternatives:* nginx GeoIP2 (accurate, but needs an ops-repo change + GeoLite2
  DB + SSR plumbing) — deferred as a possible future upgrade; Accept-Language
  parsing — rejected, English is ambiguous across US/UK/global.

### D2 — Move GA out of the inline `app.html` bootstrap

Delete the inline gtag `<script>` from `app.html` and its SHA-256 hash from the
CSP `script-src`. GA init moves into `analytics.ts` next to PostHog under a single
`initTrackers()`; the `https://www.googletagmanager.com` host stays in the CSP so
the dynamically-injected `gtag.js` still loads, and the config code now runs from
the same-origin bundle (`'self'`).

- *Why:* while GA lives inline it executes before anything can ask for consent —
  gating is impossible without this move. Post-hydration start costs a negligible
  delay for analytics accuracy and removes the brittle inline-hash coupling.
- *Alternative:* inject an "is EU" flag into `app.html` via `transformPageChunk`
  and keep GA inline — rejected as more moving parts for no benefit, and it would
  reintroduce a server-side region signal we deliberately avoid.

### D3 — Consent store as a small reactive module

`web/src/lib/consent.svelte.ts` owns `localStorage['hire.consent']` (`granted` /
`denied` / absent) and exposes reactive `$state` plus pure helpers
(`regionNeedsConsent(tz)`, `needsBanner(...)`, `grant()`, `deny()`, `reopen()`).

- *Why:* mirrors the existing guarded-wrapper posture; the pure helpers are
  unit-testable in plain Node (per the frontend testing convention), while the
  SDK/`localStorage` side effects are exercised by visual verification.

### D4 — Banner in the root layout, gated centrally

`CookieConsent.svelte` mounts in `+layout.svelte`, rendered only when
`needsBanner` is true. `initTrackers()` is called from the same central place when
`!regionNeedsConsent || consent === 'granted'`. A footer "Cookie settings" link
calls `reopen()`.

- *Why:* one decision point for both "should the banner show" and "should
  trackers start", both fed by the same consent module — no logic duplicated
  across GA and PostHog paths.

## Risks / Trade-offs

- **Timezone spoofing / VPN with mismatched TZ** → a determined EU visitor could
  present a non-EU timezone and skip the banner. Accepted: consent law targets
  good-faith detection, and the conservative default (unknown → ask) covers the
  common cases; upgrade path to GeoIP exists (D1) if ever needed.
- **Analytics starts slightly later** (post-hydration vs pre-paint) for all
  visitors → negligible for pageview/session accuracy; GA/PostHog both tolerate a
  late first event.
- **Editing the remaining inline theme script in `app.html`** still requires
  recomputing its CSP hash → unchanged risk, documented already; we only remove the
  GA hash, not the theme one.
- **Denied consent loses EU analytics** → by design and legally required; a known
  reduction in EU coverage, not a defect.

## Migration Plan

Pure frontend deploy (web rebuild). No data migration. Rollback = revert the
change; GA returns to inline boot and PostHog to unconditional init. No persisted
state beyond the per-browser `localStorage` key, which is inert if the feature is
rolled back.
