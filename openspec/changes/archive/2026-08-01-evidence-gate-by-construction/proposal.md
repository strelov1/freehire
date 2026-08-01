## Why

The evidence gate is the rule the whole tailoring capability exists to keep: a sentence
about what the candidate did cannot reach the page unless it traces to something *they*
asserted. `internal/cvedit` deliberately enforces it in the service path rather than in a
system prompt, "because a rule that lives only in a prompt is one a long conversation
eventually loses."

The rule is enforced in the right place and wired correctly today. What is wrong is that
the type system models the dependency as *optional*:

- `internal/handler/cv.go:92` builds the editor every CV write goes through with an
  explicitly nil gate: `cvedit.NewEditor(cvedit.NewRepository(pool, queries), nil)`.
- The only production attachment is a post-construction mutator, and it lives inside a
  different feature's constructor — `internal/handler/assistant.go:96`:
  `if cvH != nil && cvH.editor != nil { cvH.editor.WithEvidenceGate(...) }`.
- `PATCH /me/cvs/:id` sets `ActorAgent` for any API-key caller
  (`internal/handler/cv_tailor.go:241`). That write path has nothing to do with the
  assistant, yet it depends on the assistant's constructor having run.

So reordering handler assembly — or building the CV handlers in any context that does not
also build the assistant — silently disables the wall, with no compile error and no test
failure. The stated reason for the late binding is also false: `internal/cvedit/editor.go`
says "the bank is wired later than the CV handlers", but an equivalent, stateless
`experience.Store` is already constructed at `internal/handler/handler.go:266`,
twenty-three lines before `newCVHandlers` at `:289`.

Nothing is broken in production today. This is a latent fail-open, and the fix is to make
the dependency non-optional at the one place it is created.

## What Changes

- The experience bank is hoisted to a single variable in `handler.go` before the CV
  handlers are built, and passed into `newCVHandlers`; the editor is constructed with its
  real gate at `cv.go:92`. The bank is stateless, so the two existing constructions
  (`handler.go:266` for the experience handlers, `assistant.go:75` for the assistant)
  collapse onto the hoisted one.
- `Editor.WithEvidenceGate` is deleted. The gate becomes what it already was in
  substance — a constructor argument — and `assistant.go` stops mutating a struct it was
  merely handed.
- `bankGate.Publishable`'s `if g.bank == nil { return nil }` is deleted: a second,
  independent fail-open on the same rule, on a field that is never nil in production.
- **Not changed:** `requireEvidence`'s `if e.gate == nil` short-circuit stays, and
  `NewEditor` continues to accept a nil gate. `requireEvidence` already returns early for
  `ActorCandidate`, so a nil gate is legitimate for the candidate-only editors that
  tests construct; forcing a stub on all of them buys no invariant.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `cv-tailoring`: adds a requirement that the evidence gate is a construction-time
  dependency of the editor rather than optional wiring, so no assembly order or feature
  combination can produce an editor that admits an agent's uncited claim.

## Impact

- `internal/cvedit/editor.go` — `WithEvidenceGate` removed; `NewEditor`'s doc corrected.
- `internal/handler/handler.go` — the bank hoisted and threaded into `newCVHandlers`.
- `internal/handler/cv.go` — `newCVHandlers` takes the bank; the editor is built with the
  real gate.
- `internal/handler/assistant.go` — stops constructing a second bank and stops mutating
  the CV handlers' editor.
- `internal/handler/assistant_cv_tools.go` — `bankGate`'s nil-bank branch removed.
- `internal/handler/assistant_integration_test.go` — its harness re-did the production
  wiring by hand (`h.cv.editor.WithEvidenceGate(...)`); it now passes the gate at
  construction like production does.

No API, schema or wire-shape change. No migration.

### Found while implementing, deliberately out of scope

Three test fixtures build a `cvHandlers` literal directly — bypassing `newCVHandlers` —
with a nil gate, and then exercise the **agent** write path through it:
`cv_tailor_integration_test.go:47` (PATCH via `Bearer`, i.e. `ActorAgent`),
`assistant_cv_tools_test.go:94` (whose comment claims it "exercises the real editor —
policy, evidence gate, apply"), and the tool cases reached through them. Because they
bypass the constructor, this change does not break them — but they assert the behaviour of
a configuration production will no longer have, and `cv_tailor_integration_test.go:276`
records the divergence as intentional in a comment that contradicts how production is
wired. Recorded as a separate finding in the architecture review rather than folded in
here: reconciling it is a product question (must an API-key CV edit cite evidence?), not a
wiring one.
