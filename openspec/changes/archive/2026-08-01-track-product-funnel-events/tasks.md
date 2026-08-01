## 1. Pure helpers (test-first)

- [x] 1.1 Add a CV-upload failure-reason mapper in `web/src/lib/analytics.ts` (or
  a sibling pure module): server message → bounded reason code, with a catch-all
  for unrecognised messages. Cover the `errResumeNoText` scan-PDF message, at
  least one other rejection, and the unknown case in `analytics.test.ts`.
- [x] 1.2 Add a sign-up freshness predicate: given `User.created_at`, a clock and
  a window, decide whether the account was just created. Test the boundaries
  (inside, outside, null `created_at`).

## 2. Sign-up

- [x] 2.1 Add a once-per-account guard: a `localStorage` marker keyed by user id
  that claims the sign-up exactly once, tested for the repeat call and for storage
  being unavailable.
- [x] 2.2 Track `signup` on identity binding when the freshness predicate says the
  account is new, with `method` derived from `has_password`. One detector covers
  both registration routes — see the design note on why the `api.register()` call
  site must NOT also track.

## 3. CV upload

- [x] 3.1 Track `cv_upload` on success at the upload call site, carrying no file
  name and no résumé text.
- [x] 3.2 Track `cv_upload` on rejection, passing the server message through the
  reason mapper from 1.1.

## 4. Applications

- [x] 4.1 Find every client path that creates an application (start from
  `confirmApplied()` in `JobView.svelte`; check the inbox link action and the
  tracking board) and list them in the change before wiring.
  **Result: `markJobApplied` in `JobView.svelte` is the only one.** The other
  writers are server-side (`internal/handler/inbox_linking.go`,
  `internal/handler/gmail.go`), reached by mail linking and Gmail sync.
- [x] 4.2 ~~Track `application_created`~~ — **dropped, see design.** With one
  client path already carrying `job_track`, the event would have been a pure
  duplicate and would still have missed every server-created row it was meant to
  explain. The 2,621-vs-240 gap is answered instead: applications are created
  outside the browser.

## 5. Credit-charged features and assistant

- [x] 5.1 Track `match_run` when a job-match analysis is started.
- [x] 5.2 Track `tailor_run` when a tailoring session is started.
- [x] 5.3 Track `assistant_message` on composer submit in
  `lib/assistant/AssistantChat.svelte`, carrying no message text.

## 6. Verify

- [x] 6.1 Run the web checks (`pnpm test`, `svelte-check`) and confirm the pure
  helpers are covered.
- [x] 6.2 Deployed to prod 2026-08-01 (blue→green flip). Verified the five event
  names are present in the live client bundle under `hire-green`, and `/health`
  returns 200. Both funnels created in PostHog (EU, project 224893): "Activation
  funnel" (`$pageview` → `signup` → `cv_upload` → `match_run`) and "SEO entry
  funnel" (`job_view` → `signup`). The application funnel is not buildable from
  the browser — see 4.2.
  **Still unconfirmed by live traffic:** no `signup`/`cv_upload`/`match_run`/
  `tailor_run`/`assistant_message` had fired within 30 minutes of the flip, which
  only reflects how few sessions took those actions in that window (16 pageviews
  across 5 sessions). The funnels above are where that shows up as it arrives; a
  flat funnel after a day of traffic would mean the wiring, not the sample.
