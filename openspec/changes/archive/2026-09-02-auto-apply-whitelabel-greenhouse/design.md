## Context

`internal/atsapply`'s Greenhouse path (`Client.Submit` in client.go, `renderedHTML`/
`newBrowser` in browser.go, `ScanGreenhouseForm` in domscan.go) was built and verified
(2026-09-02 spike, `auto-apply-worker` task 7.1) against exactly one page shape: the vanilla
`job-boards.greenhouse.io/<board>/jobs/<id>` template, whose rendered form always carries
`id="application-form"` (`greenhouseFormReadySelector`, browser.go). `renderedHTML` waits up
to `pageLoadTimeout` (20s) for that selector, then hands the page to `ScanGreenhouseForm`.

A live verification run against a real posting on a white-label custom Greenhouse domain
(`careers.godaddy`, proposal.md's Why) found that page renders a form with no
`id="application-form"` at all — different field ids throughout (`form_first_name_2_3_0`,
etc.) — and is additionally gated by reCAPTCHA Enterprise widgets. `renderedHTML`'s wait times
out (confirmed against the real page even at 3x `pageLoadTimeout`), so `ScanGreenhouseForm`
is never reached; `Client.Submit` propagates a plain error, which `internal/autoapply`'s
runner cannot distinguish from a transient failure — it retries and eventually dead-letters
an attempt that could never have succeeded.

`internal/autoapply`'s `SidecarClient.Submit` contract already has an outcome built for
exactly this shape of problem: `autoapply.StatusParked` (with a `Reason` string, no `error`)
— the same outcome the existing `requiresCaptcha` short-circuit uses for Lever, and the same
one an unresolved required field uses. `Store.Park` (the runner's handling of a parked result)
does not touch the attempt's retry/dead-letter counters — see requirement 3 in this change's
spec, which this design satisfies by reusing that existing path rather than inventing a new
one.

## Goals / Non-Goals

**Goals:**
- Recognize, within a bounded and short window, that a rendered Greenhouse-provider form does
  not match the known DOM shape, and return `StatusParked` with a specific reason instead of
  an error.
- Recognize a reCAPTCHA-gated form (the concrete case found) and return `StatusParked` with a
  specific reason, without ever attempting to fill or submit.
- Do this fast: a form that will never match should not cost the full `pageLoadTimeout`
  before the attempt is given up on.

**Non-Goals:**
- Building a second DOM scanner that can actually fill/submit through GoDaddy's (or any other
  employer's) white-label form shape. One employer's bespoke id scheme does not generalize,
  and extending fill coverage to it needs its own live verification — a separate, later
  change if it turns out to be worth it.
- Detecting every possible CAPTCHA vendor. This change covers reCAPTCHA (what the live
  verification actually found); a form gated by a different challenge vendor still parks —
  see Risks — just under the more generic "unrecognized layout" reason rather than a
  captcha-specific one.
- Anything for Ashby or Lever. Lever already parks unconditionally via the existing
  `requiresCaptcha` short-circuit (before a browser is even launched); Ashby has no live
  DOM-scan path at all (`mergedFromAPIOnly`), so there is no `renderedHTML` call to harden for
  it.
- A configurable detection window. See Decisions — a fixed constant matches how
  `pageLoadTimeout` itself is already defined, and there is no operational need yet to tune
  this per-deployment.

## Decisions

**Detection strategy: a short "does the known shape exist" probe before falling back to
classification, not a longer version of the existing blind wait.** Today `renderedHTML`
issues one `chromedp.WaitVisible(greenhouseFormReadySelector, chromedp.ByID)` for the full
`pageLoadTimeout` and returns a bare timeout error either way. This change replaces that
single blocking wait with two bounded steps run against the same already-navigated page: (1)
wait for the known selector, using a shorter budget than today's full `pageLoadTimeout` — the
vanilla template's own hydration time, measured in the original spike, is well under it; (2)
on that probe's timeout (not a general error — see below), capture the page's current
`OuterHTML` (one more bounded browser call, no further waiting or JS evaluation) and
classify over that string alone: does it contain a reCAPTCHA footprint (`recaptcha` appears
in an iframe `src` or a script tag, case-insensitive), and if not, simply report
"unrecognized layout." Classifying over already-captured HTML rather than a live
`chromedp.Evaluate` keeps this pass pure and fixture-testable the same way
`ScanGreenhouseForm` already is (see Decisions below) — no live browser needed to unit-test
it. Both classification outcomes return `StatusParked`; neither triggers a second, longer
wait — a form that hasn't shown its known shape by the first probe's deadline is not given a
second chance to also fail slowly.

Alternative considered: keep one wait but raise `pageLoadTimeout` and inspect the DOM only
after it elapses. Rejected — this makes every genuinely slow-but-correct vanilla-template
attempt (the common case) pay the same higher cost as a form that will never match, for no
benefit; the two-step probe keeps the common case at its current speed and only pays the
extra classification cost on the rarer non-matching path.

**Reuse `autoapply.StatusParked`; no change to `internal/autoapply`.** The two new outcomes
(unrecognized layout, captcha-gated) both map to `SidecarResult{Status: StatusParked, Reason:
"..."}`, the exact shape the existing Lever short-circuit already produces. `Store.Park`
already exists and already excludes a parked attempt from `MaxAttempts`/dead-letter
accounting (per `auto-apply-worker`'s own design) — satisfying this change's spec requirement
3 needs no runner change at all, only a `Client.Submit` change from returning an `error` to
returning a classified `StatusParked` result for these two cases.

Alternative considered: a distinct `autoapply.SubmitStatus` value (e.g.
`StatusUnscannable`) to make the reason machine-distinguishable from an ordinary unresolved
question. Rejected for this change — `Reason` is already a free-form string the existing
Lever case uses the same way (`"requires_captcha"`), and `internal/autoapply`'s runner does
not branch on `Reason` today; adding a new enum value would be new surface with no consumer,
against this repo's no-overengineering guidance. If a future change needs to alert on this
specifically (see Risks), it can read `Reason`.

**A fixed, hardcoded detection window, not a new env var.** Matches `pageLoadTimeout`'s own
existing pattern (browser.go) — a constant tuned from real measurement, not a per-deployment
knob.

**Classification time is ADDITIVE to `pageLoadTimeout`, not carved out of it.** An earlier
version of this design shortened the known-selector wait itself (to a `formProbeTimeout`
below `pageLoadTimeout`) to make room for classification inside the same overall budget.
Live verification (task 4.2, a real, ordinary vanilla-template posting) caught this directly:
under real load the shortened wait intermittently missed the selector on a posting that
`ScanGreenhouseForm` could otherwise scan perfectly well — the exact false-positive risk this
design already anticipated, now confirmed rather than hypothetical. The fix keeps the full,
unchanged `pageLoadTimeout` for the selector wait (bit-for-bit the same as before this change
for the success path) and spends a separate, additional `classifyTimeout` (10s) only after
that genuinely elapses — the common case's timing is untouched, and only a form that has
already failed to appear "for real" pays the extra classification cost.

**Captcha-marker check stays narrow and named, matching `resolve.go`'s existing philosophy.**
The check looks for reCAPTCHA specifically (what was actually found live), not a generic
"page looks locked" heuristic — the same "never guess, only match what is concretely known"
rule this package already applies to answer resolution (see
`internal/atsapply/AGENTS.md`'s "Never guesses an answer"). A form gated by some other
mechanism still parks, just under the generic "unrecognized layout" reason instead of a
captcha-specific one — not a correctness gap, only a slightly less precise log line.

## Risks / Trade-offs

- **False positive: a slow-but-correct vanilla-template render gets classified as
  "unrecognized layout."** → This was NOT hypothetical: live verification (task 4.2) caught
  it for real on a real, ordinary posting, tracing back to an earlier version of this design
  that shortened the selector wait itself to make room for classification. Fixed by making
  classification time strictly additive to the full, unchanged `pageLoadTimeout` (see
  Decisions) rather than carved out of it, which removes the regression — the selector wait
  is exactly as generous as it was before this change existed. Residual risk (a render that
  is somehow slower than the original `pageLoadTimeout` itself already allowed for) is
  unchanged from before this change and not newly introduced by it.
- **False negative: a non-reCAPTCHA challenge (hCaptcha, Cloudflare Turnstile, etc.) is not
  named specifically.** → Still parks (via the generic "unrecognized layout" path), so no
  attempt is ever mis-submitted or mis-failed over this — the only cost is a less specific
  `Reason` string in that case, which is acceptable per this change's Non-Goals.
- **Silent accumulation: once this ships, "unrecognized layout"/"captcha" parks stop being
  visible as errors at all (by design — that is the point), so a real regression elsewhere
  (e.g. the vanilla template itself changing shape) could hide behind the same reason string
  as a truly-unscannable white-label board.** → Out of scope to build new alerting in this
  change (no existing dashboard for `atsapply` park reasons to extend), but worth flagging:
  `internal/atsapply/AGENTS.md` should note that a spike in either reason across previously-
  fillable providers is worth investigating manually, not treated as expected noise.

## Migration Plan

Pure code change — no schema, no data migration, no new configuration. Deploys like any other
`cmd/auto-apply` change: build, deploy, the next scheduled run picks it up. Rollback is a
plain revert; no data written by the old (erroring) behavior needs cleanup, since an attempt
that used to error and eventually dead-letter simply parks cleanly going forward instead —
strictly safer, not a new failure mode to unwind.
