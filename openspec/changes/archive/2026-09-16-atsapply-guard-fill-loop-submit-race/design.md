## Context

See `proposal.md` - Why for the motivation (the accepted-risk trailing `kb.Enter` on
`text`/`textarea` fields in `fillOne`, `internal/api/atsapply/fill.go:191-216`).

Relevant existing shape of `fill.go`:
- `fillAndSubmit(ctx, plan, layout)` loops `plan.Fields`, calling `fillOne(ctx, f, ...)` for
  each; on error it classifies via `classifyFillFailure(err, challengeVisible(ctx))` — note
  `challengeVisible` runs on the loop's own `ctx`, not `fillOne`'s internally-scoped one,
  since `fillOne` cancels its own context before returning.
- After the loop, it clicks `layout.submitSelector` once, then calls `verifySubmission(ctx)`,
  which polls page text for confirmation/refusal markers and reports one of three outcomes:
  confirmed (`true, nil`), refused (`false, err`), or unconfirmed (`false, nil`).
- Per `AGENTS.md`, this file's fill/submit path has **no unit tests that drive a real
  browser** — "a real browser session cannot be faked usefully." The two functions that need
  a live DOM signal but are still exercised by tests (`classifyFillFailure`,
  `pollEndedByDeadline`) take that signal as a plain parameter, so the *decision* is unit
  tested while the *live check* (`challengeVisible`) is not.

## Goals / Non-Goals

**Goals:**
- Stop the fill loop as soon as it can tell the form's submit control is gone, rather than
  continuing to fill fields on a page that may have already moved on.
- Route that case through the exact same confirmed/refused/unconfirmed classification the
  loop's own deliberate submit click already uses, so `internal/application/autoapply`'s
  runner sees no new outcome shape to handle.
- Keep the new control-flow decision unit-testable without a browser, matching this file's
  existing testing pattern.

**Non-Goals:**
- Telling a true autocomplete field apart from a plain text field. That is the actual root
  cause the existing code comment names, and per that same comment it needs live
  verification against a real board, not a guess — unchanged by this design.
- Removing the trailing `Enter` itself. It still does real, needed work (committing an
  autocomplete suggestion); this design only bounds what happens when it goes wrong.
- Any change to `select`, `checkbox_group`, or `file` handling — none of those send a key
  event that can trigger a form's own submit binding.

## Decisions

**Detection signal: submit-control DOM presence AND usability, not URL/navigation.** A new
`formStillPresent(ctx, submitSelector) bool` runs `chromedp.Evaluate`, mirroring
`challengeVisible`'s shape (same file, same "ask the page, fail closed" structure).
Considered watching for a URL change instead: rejected because these are SPA application
forms — `verifySubmission` already establishes that a real submission is detected by page
*text*, not navigation, so a URL-based signal would be inventing a second, inconsistent
detection mechanism for the same underlying event. Code review on the first cut (which
checked only `document.querySelector(sel) !== null`) found a real gap: a same-page async
(fetch/XHR) submit can leave the control mounted but `disabled`, or hidden behind a
confirmation overlay, without removing it — a bare presence check would call that "still
present" and let the loop click it a second time, the exact duplicate submission this check
exists to prevent. The check now also fails (`false`, "not usable") on `el.disabled` and on
`display: none` / `visibility: hidden`, the same visibility test `challengeVisible` already
uses for its own iframe check.

**Error default: treat an `Evaluate` failure as "gone," not "still there."**
`challengeVisible` defaults to `false` (no challenge) on error, because an error there
carries no evidence either way. This check is the opposite: the most likely reason
`chromedp.Evaluate` itself fails right after an Enter keystroke is that the keystroke just
navigated the page and destroyed the execution context — which *is* the condition being
checked for. Defaulting to "still present" would silently skip the guard in exactly the
case it exists for. Defaulting to "gone" costs, at worst, one unnecessary trip through
`verifySubmission` (which times out to `StatusUnconfirmed`, dead-lettered, not retried) when
the page was actually fine — a false stop, never a false continue. That trade matches this
package's existing bias throughout `fill.go`: prefer an honest "unconfirmed" over any path
that risks a second real submission.

**Scope the check to `text`/`textarea` only, checked once per field, in the loop, not
inside `fillOne`.** Only that branch sends `kb.Enter`. Checking runs after `fillOne`
returns successfully, in `fillAndSubmit`'s loop, using the loop's own `ctx` — the same
placement `challengeVisible(ctx)` already uses on the error path — so no new context needs
threading through `fillOne`.

**Extract a pure decision function.** `fillAndSubmit`'s loop becomes: fill field, and for a
`text`/`textarea` field, if `!formStillPresent(...)`, stop and go straight to
`verifySubmission`. The "stop or continue" branch itself is one `if`, but the *loop-level*
guarantees this change makes — every remaining field is skipped, the loop's own submit
click never fires, `verifySubmission` runs exactly once — are what needs a test. Package
this as a small helper that takes the sequence of (field kind, post-fill-presence) as
plain values (no chromedp) and returns which fields would be skipped and whether the loop's
own submit click would fire, the same "signal as a parameter" shape `classifyFillFailure`
already uses. `formStillPresent` itself stays untested, same as `challengeVisible`.

**Log the early stop unconditionally, naming how many fields actually filled.** Code review
flagged that a CONFIRMED result reached through the early-stop path can still be missing
every field ordered after the trigger field (an approved résumé included, if the DOM scan
happened to place it later in the plan) — the runner would record an ordinary
`StatusApplied` with no signal anywhere that the submission might be incomplete. This design
does not change that outcome's `Status` (inventing a fourth, "confirmed but possibly
incomplete" status is a bigger, unapproved scope change, and `outcome.filledCount` alone
cannot tell a genuinely-optional missing field from a required one skipped by the stop —
that judgment needs `Plan.Unmapped`/field-required data this loop does not carry). Instead
`fillAndSubmit` logs (`log.Printf`, this package's existing convention for a degrade-and-
continue note) `outcome.filledCount` against the plan's total field count whenever the loop
stops early, regardless of the eventual confirmed/refused/unconfirmed outcome — the
information was already being computed and discarded; surfacing it costs nothing and gives
an operator investigating a specific job a concrete signal to grep for.

## Risks / Trade-offs

- **[Risk]** A transient `Evaluate` failure unrelated to navigation defaults to "gone,"
  ending a fill that was actually fine in `StatusUnconfirmed` (dead-lettered, not retried).
  → **Mitigation**: accepted deliberately — see the error-default decision above. This is
  the same shape of trade the package already makes everywhere else in `fill.go`
  (`pollEndedByDeadline`, `verifySubmission`'s own timeout): an occasional lost attempt
  costs a retry-by-the-candidate; a missed early-submit risks a real duplicate application.
- **[Risk]** `formStillPresent` runs once, immediately after `fillOne` returns from sending
  the field's `Enter` keystroke — but the browser may not have started processing a
  triggered submission (an async `fetch`/XHR handler, client-side validation) by that exact
  moment, so the check can read "still present, enabled, visible" a beat before the page
  actually changes. → **Mitigation**: not addressed here, deliberately. Unlike the
  `Evaluate`-error case above, there is no correctness-preserving fix available without a
  guess: a fixed settle delay before checking is exactly the kind of unverified magic number
  `AGENTS.md` already warns against for this file ("a targeted fix needs live verification
  against a real board, not a guess"), and this package's own convention is to not add one
  speculatively. The residual case this leaves — the loop takes one more field's worth of
  action before a delayed real submission actually manifests — does not regress past this
  PR's baseline: that next `fillOne` call then fails against a page that has moved on,
  which is the SAME plain, ordinary error this whole class of risk produced before this
  change existed (see proposal.md's Why). This PR narrows how often that ordinary-error path
  is reached; it was never scoped to close it to zero (see the Non-Goals above).
- **[Risk]** One extra `chromedp.Evaluate` round-trip per `text`/`textarea` field lengthens
  every ordinary fill slightly. → **Mitigation**: negligible against the existing 5s
  `fillTimeout` per field; no live measurement has shown fill-loop duration as a bottleneck.
- **[Risk]** This still does not fix the underlying inability to distinguish autocomplete
  fields, so a real early submit can still happen. → **Mitigation**: explicitly a
  Non-Goal; this design only bounds the blast radius (no continued filling past the point
  of no return, no double submit click) rather than preventing the trigger. Code review
  proposed a cheaper interaction-time alternative — checking whether an open autocomplete
  listbox is visible before sending `kb.Enter`, and skipping the keystroke when none is —
  considered and deferred, not adopted here: the risk this design bounds is a form's own
  Enter-submits-the-form binding, which (per the existing code comment on `fillOne`'s
  `text`/`textarea` branch) fires from an ordinary keystroke regardless of field count, not
  specifically from a listbox interaction, so a listbox-visibility check does not reliably
  address the trigger it would be added to guard. It is also a change to `fillOne`'s
  interaction behavior itself, which this design's Non-Goals deliberately excludes pending
  live verification against a real board — the same bar the existing comment already sets
  for touching this code path at all.
