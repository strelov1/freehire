## 1. Consent module (pure logic — TDD)

- [x] 1.1 Create `web/src/lib/consent.svelte.ts` with pure helper `regionNeedsConsent(tz: string): boolean` — true for `Europe/*` + EEA Atlantic zones, and for empty/unknown input (fail toward asking). Write the failing unit test first (`web/src/lib/consent.test.ts`).
- [x] 1.2 Add consent read/write over `localStorage['hire.consent']` (`granted` | `denied` | absent) and a pure `needsBanner(region: boolean, choice: string | null): boolean` (true only when region && no choice). Unit-test `needsBanner` truth table and the stored-value round-trip guards.
- [x] 1.3 Add reactive `$state` for the current choice plus `grant()`, `deny()`, `reopen()` mutators. (State runes can't be unit-tested per the frontend convention — cover the pure branch logic in 1.1/1.2; verify the runes via the banner visual pass in group 6.)

## 2. Tracker gating (analytics.ts — TDD where pure)

- [x] 2.1 Add a guarded GA loader to `web/src/lib/analytics.ts` (inject `gtag.js` from `googletagmanager.com`, config `G-6P1PZ719T0`) alongside PostHog, exposed via a single `initTrackers(config)` that starts both and is idempotent. Keep the localhost skip that `app.html` had. Unit-test the guard/no-op branches that don't touch the DOM.
- [x] 2.2 Change the caller in `web/src/hooks.client.ts` (and/or `+layout.svelte`) to call `initTrackers()` only when `!regionNeedsConsent(tz) || choice === 'granted'`, reading region + choice from the consent module. PostHog env-gating (`PUBLIC_POSTHOG_KEY`) is preserved as an additional precondition.

## 3. CSP + inline cleanup

- [x] 3.1 Remove the inline GA `<script>` bootstrap from `web/src/app.html` (leave the anti-FOUC theme script untouched).
- [x] 3.2 Remove the now-obsolete GA inline-bootstrap SHA-256 hash from `script-src` in `web/svelte.config.js`; keep the `https://www.googletagmanager.com` host and the theme-script hash. Update the surrounding CSP comment to match.

## 4. Banner + wiring

- [x] 4.1 Create `web/src/lib/components/CookieConsent.svelte`: shown only when `needsBanner`; terminal aesthetic; equal-prominence Accept/Reject, no pre-selected default; short text + link to `/privacy`. Accept → `grant()` + `initTrackers()`; Reject → `deny()`.
- [x] 4.2 Mount `CookieConsent` in `web/src/routes/+layout.svelte` and ensure the central gate starts trackers on grant.
- [x] 4.3 Add a footer "Cookie settings" link (in the Footer component) that calls `reopen()` to re-open the banner.

## 5. Privacy policy reconciliation

- [x] 5.1 Update the Cookies section (and the "Technical data" / "How we use it" lines) in `web/src/routes/privacy/+page.svelte`: list Google Analytics and PostHog explicitly as consent-gated for EU/EEA/UK visitors, describe the banner + withdrawal, and remove the now-false "if you opt in" and "no tracking profiles" claims. Bump `lastUpdated`.

## 6. Verification

- [x] 6.1 Run the web unit suite (`vitest`) — consent pure-logic tests green.
- [x] 6.2 Visual verify via headless Chrome: (a) EU timezone → banner shows, no `_ga`/`ph_*` cookies and no gtag/PostHog network before choice; (b) Accept → both trackers load + cookies set, banner gone; (c) Reject → nothing loads; (d) non-EU timezone → no banner, trackers load; (e) footer "Cookie settings" re-opens the banner.
