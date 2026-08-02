## Why

Finding **I1**, raised while implementing S5. Three fixtures built `cvHandlers` as a struct
literal — bypassing `newCVHandlers` — with a **nil** evidence gate, then edited as `ActorAgent`.
One recorded the divergence in a comment as though it were a decision:

> the key edits as the AGENT, so a bullet has to cite banked evidence — except that this fixture
> wires no bank, which is "no gate" rather than "gate that refuses"

Production disagrees, and always has: `Register` passes `bankGate{bank}` unconditionally, so an
API-key `PATCH` inserting an uncited bullet is **403**. The fixture asserted **200**. Because it
bypassed the constructor, making the gate a construction-time argument in S5 neither fixed nor
broke it — it kept asserting a configuration that does not ship.

## The product question, answered the conservative way

The finding says to decide first: must an API-key caller editing as the agent cite banked
evidence? **Production already says yes**, in one place, unconditionally. So aligning the fixtures
changes **no product behaviour** — it changes tests that describe a configuration nobody can
reach. The alternative reading (move the gate behind a per-actor policy) *would* be a product
change, and remains a separate proposal for the owner to make.

Not settled by editing the comment, which the finding explicitly forbids.

## What Changes

- Every fixture constructs the editor with the real gate, exactly as `Register` does. Where a
  fixture genuinely has no bank it passes an **empty** one — the gate is then present and refuses,
  which is what a user with nothing banked actually experiences.
- The cases that expect a write to land now bank an atom and cite it, so they exercise
  production's success path instead of its absence.
- `TestNoEditorIsConstructedWithoutAnEvidenceGate` makes the rule enforceable: no file in the
  package may construct an editor with a nil gate. Verified to fire by restoring one.

## Three things the real gate revealed that the nil gate could not

1. **The gate is the OUTER check.** An uncited malformed op is 403, not the 422 the addressing
   check would give — so the "bad addressing is a 422" case had to cite evidence to stay about
   addressing. Both orderings are now pinned.
2. **It runs before ownership too.** A foreign caller sending a claim-bearing op is refused at the
   gate (403) and never reaches the 404. Isolation is intact — 403 is what an uncited claim gets
   whether or not the CV exists, so neither answer discloses one — but only a non-claiming op
   actually exercises the 404, so that case now uses `remove`.
3. **Eight more fixtures had nil gates** that I1 does not name. They never edit as the agent
   (`grep` finds no `doBearer`/`ActorAgent` in those files), so the gate was inert for them — but
   an agent-path case added later would have lost the wall silently. They now pass a real gate,
   which is what makes the rule test possible.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

(none) — no requirement-level behaviour change. The gate's behaviour is unchanged; the fixtures
now describe it. Archives with `--skip-specs`.

## Impact

- `internal/handler/cv_tailor_integration_test.go`, `assistant_cv_tools_test.go`,
  `cv_integration_test.go`, `cv_ats_delta_integration_test.go`, and a new rule test.
