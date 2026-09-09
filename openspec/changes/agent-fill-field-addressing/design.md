## Context

The browser-tool wire carries a fill as `{label, value}`. The page side has been able to
narrow a write for some time: `form.ts` holds `fillsForFrame` (which stops a frame-addressed
fill being broadcast) and `findQuestion` (which narrows a match to a named `<form>`), and the
panel's own walk — `planLabelFills` → `deterministicAutofill` — sets both scopes on every
fill it plans. The **agent** path does not reach any of it.

Two independent breaks put it there:

1. `internal/ai/autofillagent.Fill` carries `Frame int` with a `json:"frame"` tag and
   `agent.go` sets it from the field that justified the fill. `extension/lib/tools/executor.ts`'s
   `readFills` destructures only `{ label, value }` and returns only those, so the frame is
   dropped at the wire's receiving end. `fillsForFrame` then sees `frame === undefined`,
   which it treats as "broadcast to every frame", and `findQuestion` sees `form === undefined`,
   which it treats as "take `matches[0]`".
2. `autofillagent.Field` — the Go mirror of what `read_form` returns — has no `Form` field at
   all, so the form index the extension already sends on every field is never parsed. Even
   with break 1 fixed, the agent would have nothing to put in a fill's `form`.

The defect is invisible to the test suite because the only test that touches it,
`internal/ai/autofillagent/widget_test.go`, asserts that the Go **sender** tagged the fill
with frame 0. Nothing exercises the receiver, and the two ends are in different languages, so
no compiler or type checker spans the gap either.

## Goals / Non-Goals

**Goals:**

- An agent can address exactly the control it read, using the scopes `read_form` already
  reports.
- A fill that cannot be resolved to one control refuses instead of guessing.
- A miss tells the harness which obstacle it hit, so its next step is decidable.
- The contract is tested across the language boundary, not on either side alone.

**Non-Goals:**

- `internal/api/atsapply` (the `cmd/auto-apply` queue worker). It drives its own headless
  Chrome and addresses controls by unique element `id` inside `#application-form`, so no
  write of its can reach a second form on the page. Untouched.
- Replacing label addressing with a minted, stable field id. That is the structurally clean
  answer and it rewrites the wire for both harnesses and the panel at once; the scopes
  `read_form` already reports close the observed failure without it.
- Closing cross-frame ambiguity (see Risks).

## Decisions

### Carry both scopes, not just the frame

The frame alone closes the iframe case: an ATS application served in an iframe, with the
careers page's own job-alert signup in the top document. It does not close the case where
both forms are in the **same** document, which is the ordinary shape of a careers page that
embeds its application inline. `form.ts` already implements form-level narrowing, so the cost
of the second scope is a struct field in Go and a line in the argument reader — the page side
needs nothing.

*Alternative — frame only.* One line in `executor.ts` and no Go change. Rejected: it fixes
half of a two-shape problem and leaves the more common shape broken, and a follow-up change
would touch the same four files again.

### Refuse an ambiguous fill rather than writing the first match

Where a label matches more than one question and the fill names no form, nothing is written.

The alternative — write the first match and report a "this was ambiguous" status — keeps
today's behaviour working and tells the harness afterwards. Rejected because the harm has
already happened by then: the candidate's email is in a job-alert signup, and on many ATS
pages that signup submits independently. This also follows the grain of the codebase, which
refuses rather than guesses in the same situation elsewhere: the facet dictionaries emit
nothing for an unknown, and `cmd/auto-apply` submits only when every required question is
answered and otherwise touches nothing on the page.

The refusal is safe to add because it can only fire where the write was already a coin flip.
A fill naming a form, or a label matching one question, is unaffected.

### Add statuses to the enum rather than a free-text detail field

`executor.ts` folds the frames' outcomes with `STATUS_RANK`, a total order over the status
values — a frame that does not hold the control answers `not_found`, and that negative must
not displace the one informative answer. A free-text `detail` alongside four statuses cannot
be ranked, so the fold would have to keep an arbitrary one. Extending the enum keeps one
mechanism.

The new members slot into the existing order without disturbing it:
`not_found` < `not_fillable` < `wrong_form` < `no_option` < `ambiguous` < `deferred_combobox`
< `filled`. `ambiguous` sits above the other refusals because it is the only one a harness
can act on by re-sending.

`not_fillable` requires a small change in how the page is read, not just how it is reported:
`collectQuestions` filters disabled and hidden controls out before matching, so such a
question is currently indistinguishable from one that was never there. Producing the status
means checking for the control on the miss path, not keeping unfillable controls in the
match set — the latter would risk writing into one.

### Test the contract across the boundary, not on each side

The defect's whole shape is a Go test that verified its own caller. The new test that would
have caught it takes the JSON body the Go agent actually produces for a scoped fill and feeds
it to `readFills`, asserting both scopes survive. A Go-side fixture test and a TypeScript-side
unit test, each passing, is exactly the state the repository is in today.

## Risks / Trade-offs

**Cross-frame ambiguity stays open** → A fill naming no frame is offered to every frame; each
frame sees only its own document, so two frames each holding one match both report a clean
`filled`, and only the fold can tell there were two — after both writes landed. Closing it
needs a resolve pass before any write, which is a second round trip on every fill. After this
change the Go agent always names a frame, leaving the case to hand-written assistant calls,
so the bound is documented in `extension/AGENTS.md` and `internal/ai/browsertools/AGENTS.md`
rather than engineered around. The seam, if it is ever worth closing, is `fillByLabel`.

**The refusal could park a fill that used to land** → Only where the label matched more than
one question and no form was named, i.e. where the previous write was already arbitrary. The
agent path stops being able to hit it at all once it names both scopes. An assistant-authored
`fill_simple` can still hit it, and gets a status naming the fix.

**Three new enum members are a breaking change for the wire's consumers** → Both consumers
are in this repository (`autofillagent` and the assistant's tool path) and neither switches
exhaustively over the status today; `agent.go` compares against string constants. Nothing
outside the repo reads the wire.

**A stale extension against a new backend** → The extension is versioned and shipped
separately from the API. A new backend sending `form` to an old extension is the situation
today (the field is dropped, first match wins), so the failure mode is unchanged rather than
new. An old backend against a new extension sends no `form`, which the new extension treats
as unscoped — and now refuses on ambiguity instead of guessing, which is the safer direction.

## Migration Plan

No data migration, no schema change, no worker. Ship the extension and the API in either
order; each is correct against the other's old version, as above. Rollback is reverting the
commit.

## Open Questions

None outstanding. The two decisions that were genuinely open — whether to carry `form` as
well as `frame`, and whether an ambiguous fill writes or refuses — were settled before this
document (carry both; refuse).
