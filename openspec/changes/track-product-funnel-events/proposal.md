## Why

Analytics stops at the catalogue door. The five events we emit — `search`,
`job_view`, `job_apply`, `job_save`, `job_track` — all fire in `JobsView.svelte`
and `JobView.svelte`, so everything a signed-in user does is invisible: signing
up, uploading a CV, running a match, tailoring, talking to the assistant. The
single number that matters most for the product — 169 of 400 users uploaded a CV
— can only be counted by querying Postgres by hand.

Seventeen days of production data show what that blindness costs. There are 2,621
`job_apply` clicks against 240 rows in `applications`, and nothing explains the
gap. There are 354 rageclicks, three of them landing on the text of
`errResumeNoText` (`internal/handler/resume.go:135`) — users hammering the error
that says their PDF is a scan — and no event records how often that upload fails
or why. Every question about activation currently ends in a hand-written SQL
query, which means it is asked once and never tracked.

## What Changes

- The app emits `signup` when an account is created, carrying the method used
  (OAuth provider or password) and no PII.
- The app emits `cv_upload` on every CV upload attempt, carrying whether it
  succeeded and, on failure, a coarse reason — so the scan-PDF dead end becomes a
  number instead of a guess.
- The app emits `match_run` and `tailor_run` when those credit-charged features
  are invoked, so usage of the paid surfaces is known.
- The app emits `assistant_message` when a user sends a turn to the assistant.
- All five follow the existing rules: fired through the one analytics module,
  safe no-ops when PostHog is inert or consent is not granted, and free of PII.

Not in scope: charging for anything, changing any user-visible behaviour, and
recording assistant token cost — that last one is `meter-the-assistant-turn`,
which writes spend to Postgres for billing. This change records *that a turn
happened* for product analytics; the two do not overlap.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `product-analytics`: the "Explicit funnel events" requirement currently names
  five catalogue events as the core funnel. It changes to also require the five
  account-side events above, so the funnel spans the whole product rather than
  stopping at the job list.

## Impact

- `web/src/lib/analytics.ts` — no API change; `track()` already accepts an
  arbitrary event name and props.
- The call sites: the sign-up/OAuth completion path, the CV upload component,
  the application-tracking action, the match and tailor entry points, and the
  assistant composer.
- No backend change, no new environment variable, no schema change.
- PostHog dashboards gain two funnels: activation (visit → `signup` →
  `cv_upload` → `match_run`) and SEO entry (landing on `/jobs/:slug` →
  `job_view` → `signup`). Those are configured in PostHog, not in code. An
  application funnel was intended as a third; tracing the write paths showed it
  cannot be built from the browser — see the design note.
