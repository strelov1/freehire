## Why

An agent filling an application form through the browser-tool wire cannot say *which*
control it means. It reads fields tagged with the frame and form they live in, and then
addresses its writes by label text alone — so a page carrying the same label twice takes
the first match. On a careers page that is routinely the job-alert signup rather than the
application, and the wrong write is reported as `filled`.

The frame half of the fix was already written and never connected. `autofillagent.Fill`
serialises `"frame"` on every fill and a Go test asserts it does; the extension's
`readFills` reads only `{label, value}` and drops it. `form.ts` then sees a fill naming no
frame, broadcasts it to every frame, and matches the first question carrying the label —
exactly the behaviour the frame tag exists to prevent. The Go test is green because it
checks the sender, not the receiver: nothing tests the contract across the language
boundary, so the field has been silently discarded since it was introduced.

## What Changes

- The fill wire carries **both** scopes. `fill_simple` accepts `frame` and `form` on each
  fill; `read_form` already reports both on every field, so an agent can name exactly the
  control it read.
- Go learns the form index it is already being sent: `autofillagent.Field` gains `Form`,
  and `Fill` carries it through from the field that justified the fill.
- The extension stops discarding scope: `readFills` reads `frame` and `form` off the wire.
- **A fill that cannot be resolved to one control writes nothing.** Where a label matches
  more than one question and the fill names no form, the outcome is `ambiguous` and the
  page is untouched — a guess here writes a candidate's details into a form they did not
  choose.
- The outcome vocabulary names the obstacle instead of collapsing every miss into
  `not_found`: `ambiguous`, `wrong_form` (the label exists, but not in the form named) and
  `not_fillable` (the control was found but is disabled or hidden, which today is
  indistinguishable from absent).
- **BREAKING** for the wire's two in-process harnesses only: `FillStatus` gains three
  members, so anything switching exhaustively over it must handle them. No stored data and
  no public API changes.

## Capabilities

### New Capabilities

- `browser-tool-form-fill`: the `fill_simple` primitive's end of the browser-tool wire —
  how a fill addresses one control (label, frame, form), the refusal when it cannot, and
  the outcome vocabulary a harness reads back. Sibling to `browser-tool-page-read`, which
  covers `read_page`.

### Modified Capabilities

None. `extension-autofill` governs which contact values are offered, not how a write is
addressed, and `browser-tool-page-read` covers a different primitive.

## Impact

- `extension/lib/tools/executor.ts` — `readFills` reads scope; `STATUS_RANK` ranks the new
  statuses when folding frames' answers.
- `extension/lib/protocol.ts` — `FillStatus` gains three members.
- `extension/lib/form.ts` — `fillByLabel`/`findQuestion` produce them; ambiguity refuses.
- `internal/ai/autofillagent/agent.go` — `Field.Form`, `Fill.Form`, and the carry-through.
- `extension/AGENTS.md`, `internal/ai/browsertools/AGENTS.md` — the addressing rule and the
  one bound this change does not close.

**Out of scope — `internal/api/atsapply` does not have this defect.** The auto-apply queue
worker drives its own headless Chrome and addresses controls by unique element `id` within
`#application-form`, not by label across a whole page, so no second form on the page is
reachable by one of its writes.

**A bound this change does not close, and documents instead:** cross-frame ambiguity. A
fill naming no frame is offered to every frame, and each frame sees only its own document —
so two frames each holding one match both report a clean `filled`, and only the merge can
see there were two. The merge runs after the writes land. After this change the Go agent
always names a frame, leaving the case to hand-written assistant calls; closing it properly
means a two-phase resolve, which is not worth building for it today.
