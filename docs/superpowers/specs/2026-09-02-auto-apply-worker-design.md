# Auto-apply worker: submit a job application unattended

Date: 2026-09-02 (revised same day, after a second spike)
Status: proposed

## Problem

`internal/applyform` captures the questions an ATS form asks, and `internal/autofillagent` /
`RunAgentAutofill` can fill whatever page the user is currently looking at through the
browser extension — but both need the candidate present. Nothing in the system can take a
job the candidate has already been matched to and actually submit an application without
them opening a tab.

A spike (2026-09-02, throwaway, not committed) confirmed two things against a live
Greenhouse posting (`webflow`, job `7951430`):

- The rendered application form carries 36 fields; the `?questions=true` API `applyform`
  reads today declares 17. The gap is exactly what a third-party reference implementation
  (ApplYourself) measured: `country`, `candidate-location` (declared as `location` — a name
  mismatch), and the whole EEOC block (`gender`, `hispanic_ethnicity`, `veteran_status`,
  `disability_status`) are rendered but never declared.
- `patchright` (a Playwright fork that patches CDP-level automation leaks) flips
  `navigator.webdriver` from `true` to `false` at zero code cost over stock Playwright, but a
  public bot-detection page (bot.sannysoft.com) still fails several other headless tells
  (`window.chrome` object missing, permissions API shape, `HEADCHR_*` checks) under both
  drivers. Patchright closes one specific, commonly-checked signal; it is not a complete
  stealth solution out of the box.

A second spike, same day, tested whether Python was actually earning its keep: `chromedp`
(pure Go, no separate process or language) against the same bot.sannysoft.com page, stock
and with one launch flag (`disable-blink-features=AutomationControlled`). Stock chromedp
against a **real installed Chrome** already passed `window.chrome`, `HEADCHR_CHROME_OBJ`,
`HEADCHR_PERMISSIONS`, `HEADCHR_PLUGINS` and `HEADCHR_IFRAME` — checks Patchright's bundled
`headless_shell` failed — because those checks largely detect "this is a stripped-down
headless build", not "this is automated", and chromedp was driving the real browser rather
than Playwright's minimal bundle. With the one flag added, chromedp also passed `WebDriver`,
matching Patchright's one fix. Net result: chromedp + a real Chrome install matched or beat
Patchright on every signal this test checks, with no second language, no second process, and
no new deploy artifact. (Caveat: this conflates two variables — driver library and browser
binary/channel; Playwright can also be pointed at a real Chrome via `channel: "chrome"`,
untested here. The practical conclusion — chromedp is not the weaker option it was assumed
to be — holds regardless.) This revision drops the Python sidecar; see the sections below.

## Scope of this design

This is the first of four dependent pieces toward full auto-apply (DOM-scan+reconcile
persistence, richer answer resolution, and the extension fallback for parked applications are
the other three). It is deliberately narrow:

**In scope:** a worker that, given a job and a candidate's existing profile answers, either
submits a real application or leaves it untouched and says why not.

**Out of scope, decided explicitly:**
- **What populates the queue** — whether a candidate clicks "Auto Apply" on one posting, or a
  standing per-user rule queues anything above a score threshold. The worker only drains
  whatever is there.
- **DOM-scan persistence** — `internal/applyform`'s stored schema exists for "the consumer
  that fills a form rather than describing it" (its own docs), implying a reconciled schema
  saved once and reused. This design scans the live DOM inside the same browser session used
  to fill it, on every run, and stores nothing new. Cheaper to build first; revisit if
  repeated scanning becomes a cost or reliability problem.
- **Retrying a parked application after the candidate adds the missing answer** — for now
  that is "someone flips the queue row back to pending"; no automatic re-trigger.
- **Routing a parked application into the extension's `RunAgentAutofill`** as a guided
  manual finish. Natural next step, not this piece.

## Architecture

```
cmd/auto-apply (Go, cron, run-once-and-exit)
  └─ internal/outbox.RunPool over auto_apply_queue
        for each claimed (user_id, job_id):
          profile := candidateprofile.Assembler.Assemble(user_id) → .Fields()  [already built]
          job     := job's apply_form URL + provider
          internal/atsapply.Submit(ctx, jobURL, provider, answers)   [in-process, chromedp —
          ─────────────────────────────────────────────────────────  no sidecar, no second
            scan DOM → reconcile w/ API schema (GH/Ashby only) →      language, no second
            resolve fields against `answers` → all required           deploy artifact]
            resolved? → fill → submit → verify by text marker
                      : else → touch nothing
          ─────────────────────────────────────────────────────────
          ← SidecarResult{Status: applied|parked, Unmapped: [...]} | error

          applied  → dbStore.Submit (LockJobForApply + MarkJobApplied  [already built,
                     + retire queue entry, one transaction)              unchanged by
          parked   → dbStore.Park (blocked_at + unmapped reasons,        this revision]
                     user_jobs.stage untouched)
          error    → retry via the queue's own lease/attempts,
                     same mechanics as cmd/capture-apply-form
```

Revised from the original Python-sidecar version of this design (see "Problem" above): a
second spike found `chromedp` + a real Chrome install matches or beats Patchright on every
signal the first spike's bot-detection check covers, and a follow-up check found neither
Greenhouse, Lever nor Ashby exposes a submission API this project could call directly —
Greenhouse's does exist but requires the *employer's own* Job Board API key, which a
third-party platform never has. So the sidecar/HTTP/Python boundary this design originally
proposed bought nothing: same DOM-driving requirement, worse stealth than the Go-native
option, plus a second language and deploy artifact. `internal/autoapply`'s
`SidecarClient` interface (already built) does not change — only what implements it does.

### Why a live scan instead of the persisted `applyform` store

`internal/applyform` today stores only the API-declared questions, which the spike showed is
incomplete for Greenhouse and is DOM-only for Lever entirely (no question API at all). Reading
that store as-is would silently under-fill required fields. Rather than building the
persistence-and-reconciliation layer first, the sidecar scans the real DOM at fill time, in
the same browser session — it already has to open the page to fill it, so the scan is nearly
free there. The persisted, reconciled schema `applyform` anticipated stays a real
improvement (faster, cacheable, reusable by the job-page display that already reads this
store) — just not a prerequisite for a working worker.

### The browser-driver boundary

Everything ATS-specific — DOM structure, react-select-equivalent verification, upload
confirmation, submission text markers, chromedp launch/stealth configuration — lives in a
new Go package, `internal/atsapply`, mirroring the reference implementation's own module
boundary in spirit (plan decides, fill executes, nothing decides twice) even though it is no
longer a separate process. `internal/autoapply`'s `Run` loop never inspects a form field; it
only assembles `answers` from data it already has and interprets `atsapply`'s result.

`answers` is exactly `candidateprofile.Profile.Fields()`'s output — the same map
`internal/candidateprofile`'s `Assembler` already builds for the extension-autofill path
(full name, contacts, work authorization, salary/notice-period facts; see the "extracted
`internal/candidateprofile`" note added to the OpenSpec task list). No new answer source for
this piece; `atsapply`'s resolver only ever sees Tier A (identity) / Tier B (work
authorization) data. A form whose required fields extend past that set always comes back
`parked` in v1 — there is no Tier C (LLM-drafted answer) yet.

`chromedp` needs a real Chrome/Chromium binary on the host, same as Patchright would have
needed its own bundled browser — no new class of dependency, just one fewer runtime
(no Python venv, no pip install) alongside it.

## Data model

New table, `auto_apply_queue`, shaped after `applyform`'s capture queue:

- `user_id`, `job_id` — composite identity of one attempt.
- `status` — `pending` / `blocked` / `done` / `failed`. (Not `user_jobs.stage` — see above.)
- `attempts`, `last_error`, `claimed_until` — the same lease/retry fields
  `internal/outbox.RunPool` already expects, so the worker is a thin `cmd/` wrapper, not new
  concurrency-control code.
- `unmapped` (jsonb, nullable) — the `[{id, label, required, reason}]` list from a `parked`
  result, for whatever later reads this to route into the extension fallback.

Population (`INSERT`) and requeue (`status` reset to `pending`) are both out of scope here, as
decided above — whatever populates the queue writes directly to this table.

## Error handling

- **Browser launch or navigation failure / timeout** — treated as `failed`, retried through
  the queue's lease mechanics, same shape as a transient `applyform` capture failure. (No
  more "sidecar unreachable": there is no second process to lose contact with.)
- **Board requires a captcha** (Lever renders one on every posting) — `atsapply` reports
  `parked` with a `requires_captcha` reason rather than attempting a blind submit; this board
  therefore parks every single time in v1, which is expected and not a bug to chase here.
  Solving it interactively is extension-fallback territory (out of scope).
  Ashby/Greenhouse aren't known to gate on one, but a run that hits one there reports the same
  way rather than guessing.
- **A required field the API declares but the DOM never renders, or vice versa** — the DOM is
  authoritative (per the spike and the reference implementation's own measurement); a
  declared-but-absent field is simply not in the resolver's input at all.
- **Partial fill state on failure** — `atsapply` only ever fills after confirming every
  required field resolves; a `parked` result never touches the page, so there is no partial
  fill to clean up. A `failed` result (crash mid-fill, after the resolved-check passed) may
  leave a half-filled, unsubmitted browser page, but the browser context is torn down either
  way — nothing persists past one attempt.
- **No submission API to fall back to** — checked directly: Greenhouse documents a
  `POST /v1/boards/{token}/jobs/{id}` application-submit endpoint, but it requires the
  *employer's own* Job Board API key (Basic Auth) — meant for that employer's own careers
  site to post into their own account, not for a third party. Lever and Ashby expose no
  submission endpoint at all. So DOM automation is not a stand-in for a simpler path; it is
  the only path available to a platform that is not the employer.

## Testing

- Go side: `cmd/auto-apply`'s queue-claim loop tested against a mock `atsapply`/
  `SidecarClient` implementation, no browser in unit tests — same discipline as
  `applyform`'s runner tests (already built this way; unaffected by this revision).
- `internal/atsapply`: unit tests over fixture HTML per provider (mirroring the reference
  implementation's `--mock` approach) for the scan → reconcile → resolve chain, with no live
  network or browser. A separate, manually-run check against live postings (as the spike
  did) stays outside CI, the same way `applyform`'s live-fetch code is exercised manually
  today.
- No test asserts a real submission succeeds against a live board — that would spam a real
  employer's pipeline. Confirmation-marker logic is tested against captured fixture HTML of a
  real confirmation page, not a live run.

## Open risks (carried forward, not solved here)

- **Datacenter IP reputation** is a separate signal from browser fingerprint; neither
  Patchright nor chromedp's stealth flag addresses it. The worker running from the same
  hosts as other cron jobs may get blocked independently of how clean the browser looks. A
  residential/mobile proxy would mitigate this — routing chromedp's traffic through one is a
  config change (`HTTP_PROXY` on the launch options), not an architecture change — but it is
  a real recurring cost, so it stays undesigned until `parked`/`failed` rates from actual
  runs show whether it is needed, rather than being built against a guess.
- **`CHR_MEMORY` and `HEADCHR_UA` fail under chromedp too**, same as they did under
  Patchright (both spikes measured this against bot.sannysoft.com). Neither test isolates
  which specific signal a given ATS's own bot-detection actually keys on, so further
  hardening — if any board's block rate shows it is warranted — is still open work, just no
  longer scoped to a Python-specific fix.
