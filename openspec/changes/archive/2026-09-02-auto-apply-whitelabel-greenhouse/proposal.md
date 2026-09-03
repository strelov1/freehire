## Why

A live verification run (2026-09-02, against the real GoDaddy Greenhouse posting
`godaddy:7753818003`, `https://careers.godaddy/jobs?gh_jid=7753818003`) found that
`internal/atsapply`'s DOM-scan was only ever verified against the vanilla
`job-boards.greenhouse.io/<board>/jobs/<id>` template (`auto-apply-worker`'s 2026-09-02 spike
and task 7.1 smoke test). GoDaddy's board is configured behind a white-label custom domain
instead — `job-boards.greenhouse.io/godaddy/...` 302-redirects to `careers.godaddy`, so the
vanilla template is unreachable for this employer at all. The custom-domain page renders a
completely different DOM id scheme (`form_first_name_2_3_0`, `form_field_kind_2_3_0`, ... —
no `id="application-form"` anywhere) and is gated by reCAPTCHA Enterprise widgets on the form
itself, which `requiresCaptcha` (client.go) does not know about (it lists only `"lever"`
today). The practical result: `renderedHTML`'s selector wait never succeeds (confirmed against
the real page with a 60s wait, well past the package's real 20s `pageLoadTimeout`), so
`ScanGreenhouseForm` is never reached and `Client.Submit` returns a hard error rather than the
"correctly declined to guess" `StatusParked` the rest of the package is designed around —
`internal/autoapply`'s runner then treats it as a transient failure (retry budget, eventual
dead-letter) instead of recognizing it can never succeed. Large/enterprise employers commonly
configure a custom career-site domain for their Greenhouse board, so this is a likely-common
production case, not a one-off.

## What Changes

- Detect, before spending a full `pageLoadTimeout` window waiting on a selector that will
  never appear, that a Greenhouse posting's rendered form does not match the vanilla
  DOM shape `ScanGreenhouseForm`/`greenhouseFormReadySelector` were built for — so a
  white-label custom domain fails fast and cleanly rather than timing out.
- Extend the captcha-park reasoning `requiresCaptcha` already applies to Lever (park before
  ever launching a fill/submit path) to a reCAPTCHA widget detected on a Greenhouse posting's
  own rendered form, rather than only trusting a static per-provider table that cannot express
  "some Greenhouse boards, not others."
- Give `internal/atsapply.Client.Submit` a distinct, honest outcome for "this posting's form
  could not be scanned at all" (unrecognized DOM shape, captcha-gated, or otherwise) — parked
  with a reason, not a bare error — so `internal/autoapply`'s runner does not burn the normal
  retry/dead-letter budget on an attempt that can never succeed no matter how many times it is
  retried.
- Explicitly **not** in scope: building a second DOM scanner that actually fills and submits
  through a white-label custom-domain form. This change is about recognizing and cleanly
  declining the case, not extending fill coverage to it — see design.md's Non-Goals for the
  reasoning (a bespoke-per-employer DOM shape is not something a single scanner can safely
  generalize to without per-employer verification, which is out of scope here).

## Capabilities

### New Capabilities
- `atsapply-unscannable-form-detection`: recognizing, before a submission is attempted, that
  a job posting's application form cannot be safely scanned or filled by this package's
  existing Greenhouse driver (an unrecognized DOM shape such as a white-label custom domain,
  or a reCAPTCHA-gated form) — and reporting that as a clean parked outcome with a specific
  reason, rather than a plain error that looks like a transient, retryable failure.

### Modified Capabilities
(none in `openspec/specs/` — `auto-apply-submit` was proposed by `auto-apply-worker`, which is
not archived yet, so there is no main spec to delta against; the same situation
`auto-apply-llm-drafting` documented. This change's behavior change to `Client.Submit`'s error
path — recognize an unscannable form and park it, rather than surfacing a bare error — should
be reconciled into `auto-apply-submit`'s own requirements when `auto-apply-worker` archives.)

## Impact

- **`internal/atsapply`**: `browser.go` (form-shape/captcha detection ahead of the selector
  wait), `client.go` (`requiresCaptcha`'s reasoning widened past a static per-provider table;
  `Submit`'s error path for an unscannable form now returns a parked result), possibly
  `domscan.go` if a lightweight "does this look like our expected shape" check belongs there.
- **`internal/autoapply`**: `runner.go`'s handling of a parked-for-unscannable-form outcome —
  confirming it degrades the same way an ordinary parked attempt does (no dead-letter, no
  wasted retry budget), not a new special case in the runner itself.
- **No new tables, no new dependencies.** Reuses the existing `SidecarResult`/`StatusParked`
  vocabulary `internal/autoapply.SidecarClient` already defines.
- **Operational**: reduces false dead-letters for postings on a white-label Greenhouse domain,
  which `auto-apply-worker`'s live smoke test suggests is common among larger employers.
