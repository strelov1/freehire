## Why

Google Analytics (inline in `web/src/app.html`, allow-listed in the CSP) and
PostHog with session recording (`web/src/lib/analytics.ts`) both start on page
load with no consent mechanism and no banner. For EU/EEA/UK visitors this
violates ePrivacy/GDPR, which require prior opt-in before non-essential trackers
run. The privacy policy compounds the gap: it claims cookies load "if you opt in"
(there is no opt-in) and that we use "no tracking profiles" (GA + session replay
are exactly that). We need a consent gate that blocks both trackers until the
visitor agrees — shown only where it is legally required.

## What Changes

- Detect whether a visitor is in a consent-required region (EU/EEA/UK) from the
  browser timezone — no IP geolocation, no nginx/ops change.
- Persist the visitor's choice (`granted` / `denied`) in `localStorage` and gate
  both trackers on it.
- **BREAKING (behavioral):** Google Analytics no longer boots from the inline
  `app.html` bootstrap. GA init moves into `analytics.ts` alongside PostHog
  behind a single `initTrackers()`, and its now-obsolete inline SHA-256 hash is
  removed from the CSP `script-src`.
- Neither GA nor PostHog initializes for a consent-required visitor until they
  click **Accept**; a **Reject** loads nothing. Visitors outside the region keep
  analytics-on-load with no banner.
- Add a `CookieConsent` banner (root layout) with equal-prominence Accept/Reject
  buttons, no pre-selected default, and a link to the privacy policy.
- Add a footer "Cookie settings" entry point to re-open the banner so consent can
  be withdrawn as easily as it was given.
- Fix the privacy policy Cookies section: list GA and PostHog explicitly, describe
  the consent mechanism, and remove the now-false "if you opt in" and "no tracking
  profiles" claims.

## Capabilities

### New Capabilities

- `cookie-consent`: region detection (timezone-based), consent persistence and
  state, the banner UI and its withdrawal entry point, and the gating of every
  non-essential tracker (GA + PostHog) behind a granted consent — for
  consent-required visitors only.

### Modified Capabilities

- `product-analytics`: PostHog initialization gains a second precondition. Today
  it initializes whenever `PUBLIC_POSTHOG_KEY` is set; it must now also require
  that a consent-required visitor has granted consent (visitors outside the
  region are unaffected).

## Impact

- Frontend only (`web/`); no backend, no API, no database, no ops/nginx change.
- Touched files: `web/src/app.html` (remove inline GA), `web/svelte.config.js`
  (drop GA inline hash from CSP), `web/src/lib/analytics.ts` (add GA + gated
  `initTrackers()`), `web/src/hooks.client.ts` and `web/src/routes/+layout.svelte`
  (wire the gate + mount the banner), plus new `web/src/lib/consent.svelte.ts` and
  `web/src/lib/components/CookieConsent.svelte`, and edits to
  `web/src/routes/privacy/+page.svelte` and the footer component.
- Existing GA analytics on non-EU traffic is unchanged in coverage; EU traffic
  that rejects consent drops out of GA and PostHog by design.
