## Why

`fillOne` (`internal/api/atsapply/fill.go`) sends a trailing `kb.Enter` after typing into
every plain `text`/`textarea` field, meant as a no-op on an ordinary field and a suggestion-
commit on a react-select-backed autocomplete field (country, candidate location) that this
package's DOM scan cannot currently tell apart from a plain text field. A code review already
named the risk this carries and left it unaddressed: a React-driven SPA form can bind
Enter-submits-the-form behavior regardless of field count, so that Enter can trigger a REAL
submit in the middle of `fillAndSubmit`'s fill loop — before every field is filled and before
the loop reaches its own, deliberate submit click. The fill loop has no way today to notice
this happened; it keeps filling fields that may no longer exist on the page and then clicks a
submit control that may already be gone, on an application that may have already gone out
incomplete.

## What Changes

- After each `text`/`textarea` field fill, check whether the form's own submit control is
  still present on the page. If it is gone, treat the submission as already possibly under
  way: stop filling the remaining fields, skip the loop's own submit click, and go straight
  to `verifySubmission` to find out what actually happened — the same honest
  confirmed/refused/unconfirmed classification `fillAndSubmit` already uses for its ordinary
  submit click.
- Extract the loop's stop/continue decision into a small function that takes the
  presence-check result as a plain `bool`, so the control flow (stop early, never double
  click submit, always end in exactly one `verifySubmission` call) is unit-testable without a
  real browser — mirroring how `classifyFillFailure` and `pollEndedByDeadline` already take
  their browser-observed signal as a parameter rather than driving chromedp inside the test.
- The live DOM presence check itself (`formStillPresent`, a `chromedp.Evaluate` call) stays
  untested by unit tests, exactly like its sibling `challengeVisible` — a real browser session
  cannot be faked usefully, per this package's own documented testing stance.
- No change to `select`, `checkbox_group`, or `file` field handling — none of those send a
  key event that can trigger a form's own submit binding, so the added check is scoped to the
  one field kind that carries the risk.

## Capabilities

### New Capabilities
- `atsapply-fill-submit-safety`: guards the fill loop against an early, accidental submit
  triggered mid-fill, so a submission that may already be underway is verified rather than
  double-submitted or silently abandoned.

### Modified Capabilities
(none — no existing capability spec describes fill-loop/submit-click behavior; see
`atsapply-form-layouts` and `atsapply-browseruse-fallback` for the closest adjacent
capabilities, neither of which covers this.)

## Impact

- `internal/api/atsapply/fill.go`: `fillAndSubmit`'s loop, plus a new `formStillPresent`
  helper and a new pure decision function.
- `internal/api/atsapply/fill_test.go` (new or extended): unit tests for the extracted
  decision function.
- No API, schema, or queue changes. `internal/application/autoapply`'s runner is unaffected —
  this change only decides what `fillAndSubmit` does before it returns one of the outcomes
  the runner already handles (confirmed / refused / unconfirmed).
