## 1. Detection primitives (`internal/atsapply/browser.go`)

- [x] 1.1 Split `renderedHTML`'s single blocking `WaitVisible` into two steps: the full,
      UNCHANGED `pageLoadTimeout` wait for `greenhouseFormReadySelector` first, then — only
      on that wait's own timeout — one `OuterHTML` capture of the page as it currently
      stands. Revised from the original plan during implementation: an initial version
      shortened the selector wait itself to make room for classification inside the same
      budget, and live verification (4.2) caught that as a real false-positive regression on
      an ordinary vanilla-template posting — see design.md's Decisions/Risks for the fix
      (classification time is now additive, via a separate `classifyTimeout`, not carved out
      of `pageLoadTimeout`). The success path (selector found) is behaviorally and
      timing-wise unchanged from before this change.
- [x] 1.2 Add a pure, fixture-testable classifier over the captured HTML string: a reCAPTCHA
      footprint (`recaptcha` appearing in an iframe `src` or a script tag, case-insensitive)
      maps to `reasonCaptchaProtected`; absent that, `reasonUnrecognizedLayout` as the
      fallback. Return a typed classification (not a bare error) so `Client.Submit` can map
      it to a `Reason` string.
- [x] 1.3 Pick `classifyTimeout` (the follow-up capture's own budget, additive to
      `pageLoadTimeout`) as a fixed constant, no new env var, per design.md's "no
      configurable window" decision.

## 2. Wiring into `Client.Submit` (`internal/atsapply/client.go`)

- [x] 2.1 On a classification result from 1.1-1.2 (as opposed to a genuine navigation/browser
      error, which still propagates as today), return `autoapply.SidecarResult{Status:
      autoapply.StatusParked, Reason: "unrecognized_form_layout"}` or `Reason:
      "form_captcha_protected"` instead of the current bare `fmt.Errorf("render application
      page: %w", err)`. No change to `internal/autoapply` — reuses the existing `StatusParked`
      path per design.md.
- [x] 2.2 Confirm (by inspection, and in the unit tests below) that a genuine transient
      failure — a network error, a browser crash, anything that is not "the known selector
      never appeared" — is unaffected and still returns an `error` through the ordinary
      failure/retry path. The two must not be conflated.

## 3. Unit tests

- [x] 3.1 Unit test the pure classifier over fixture HTML strings, the same pattern
      `domscan_test.go` already uses: HTML containing a reCAPTCHA iframe/script →
      `reasonCaptchaProtected`; plain HTML with neither → `reasonUnrecognizedLayout`. No live
      browser needed — this is the whole point of classifying over captured HTML rather than
      a live `chromedp.Evaluate` (see design.md's Decisions).
- [x] 3.2 Test `Client.Submit`: both new classifications map to `StatusParked` with the
      expected `Reason` and no error; a genuine navigation error still returns a plain `error`
      (regression guard for 2.2).
- [x] 3.3 Test (or extend an existing `internal/autoapply/runner_test.go` case) that a
      `StatusParked` result — any `Reason` — never touches `Fail`/dead-letter accounting; this
      should already hold given `auto-apply-worker`'s existing `Store.Park` behavior, but add
      a case naming these two reasons explicitly so a future change to that mapping cannot
      regress it silently.

## 4. Live verification

- [x] 4.1 Re-ran the same live-verification driver used to find this gap (throwaway, not
      committed — mirrors `auto-apply-worker` task 7.1's own precedent) against the real
      GoDaddy posting (`godaddy:7753818003`, `careers.godaddy`) that surfaced it. Confirmed,
      reproducibly across repeated runs: `Client.Submit` now returns `StatusParked` with
      `Reason: "form_captcha_protected"` (this posting's actual blocker) instead of erroring,
      in ~24s (well inside the `pageLoadTimeout` + `classifyTimeout` budget) rather than
      timing out.
- [x] 4.2 Spot-checked real, currently-open vanilla `job-boards.greenhouse.io` postings
      (Captify `8778315002`, Enova `8174624`) end-to-end through the same code path. **Found
      and fixed a real regression this way**: an earlier version of 1.1 (a shortened
      `formProbeTimeout` carved out of `pageLoadTimeout`) intermittently misclassified one of
      these ordinary, fully-scannable postings as `unrecognized_form_layout` under real load
      — exactly the false-positive risk design.md's Risks had flagged as a residual,
      "accepted" risk, except it was not actually residual, it was a live bug in that first
      cut. Fixed per 1.1's revision (classification time additive, selector wait unchanged);
      re-verified three consecutive fresh runs (`-count=1`, cache bypassed) against both
      postings with the fix in place, all resolving normally (ordinary unmapped-required-
      field parks — résumé upload, unanswered custom questions — the same outcome this
      package already produced before this change existed).

## 5. Documentation

- [x] 5.1 Update `internal/atsapply/AGENTS.md`'s "Always true" section: note that a Greenhouse
      posting on a white-label custom domain, or one whose form is challenge-protected, parks
      with a named reason rather than erroring — and fold in design.md's Risks note that a
      spike in either reason among previously-fillable providers is worth investigating
      manually (possible regression in the vanilla template itself), not treated as expected
      noise.
- [x] 5.2 Note in the same file (or wherever `auto-apply-worker`'s own live-smoke findings are
      recorded) that this change's scope is detection/parking only — it does not add fill/
      submit coverage for white-label Greenhouse domains, per design.md's Non-Goals.
