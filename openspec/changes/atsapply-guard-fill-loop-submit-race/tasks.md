## 1. Pure loop-decision logic (test-first, no browser)

- [x] 1.1 Write failing unit tests in `internal/api/atsapply/fill_early_submit_test.go` for
      the fill loop's stop/continue decision, driven by plain `bool` presence results (no
      chromedp): a `text`/`textarea` field followed by a "gone" presence result skips every
      remaining field and never reaches the loop's own submit click; a "still present"
      result continues to the next field exactly as today; a sequence where the presence
      result is never "gone" reaches the loop's own submit click exactly once.
- [x] 1.2 Implement the minimal decision function/type in `fill.go` to make 1.1 pass,
      matching design.md's "extract a pure decision function" approach (signal-as-parameter,
      same shape as `classifyFillFailure`).

## 2. Live submit-control presence check

- [x] 2.1 Add `formStillPresent(ctx context.Context, submitSelector string) bool` to
      `fill.go`, structured like `challengeVisible` (a `chromedp.Evaluate` checking the
      selector resolves to an element that is present, not `disabled`, and not hidden via
      `display: none` / `visibility: hidden` — widened from a bare
      `document.querySelector(sel) !== null` after code review found a same-page async
      submit can leave the control mounted but unusable), defaulting to `false` ("gone") on
      an `Evaluate` error — document the reasoning inline (an error here is itself likely
      evidence of a destroyed execution context, i.e. a navigation), per design.md's
      "Error default" decision. No unit test for this function itself, matching
      `challengeVisible`'s existing precedent — note that explicitly in its doc comment.

## 3. Wire the guard into `fillAndSubmit`

- [x] 3.1 Update `fillAndSubmit`'s loop: after a `text`/`textarea` field fills
      successfully, call `formStillPresent(ctx, layout.submitSelector)`; when it reports
      "gone," stop the loop (no further `fillOne` calls, no loop-owned submit click) and
      return `verifySubmission(ctx)` directly. Leave `select`, `checkbox_group`, and `file`
      handling untouched — no presence check follows those field kinds.
- [x] 3.2 Update `fillOne`'s existing doc comment on the `text`/`textarea` branch
      (`fill.go:198-211`, "known, accepted risk... not worked around blind") to describe
      the risk as now bounded by this guard rather than fully unaddressed — the trigger
      (an unrecognized autocomplete field) is still unfixed, but a triggered early submit
      no longer causes further blind filling or a second submit click.

## 4. Verify

- [x] 4.1 `gofmt -l internal/api/atsapply` prints nothing.
- [x] 4.2 `go vet ./internal/api/atsapply/...`
- [x] 4.3 `go test ./internal/api/atsapply/...`
- [x] 4.4 Re-read `internal/api/atsapply/AGENTS.md`'s "least-verified part of this package"
      paragraph; update it only if this change makes any sentence there inaccurate (it
      should still say the fill/submit path has no live-browser test coverage — this change
      does not add any — but should not go on describing the Enter risk as entirely
      unaddressed once 3.2 lands).
