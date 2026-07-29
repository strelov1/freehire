## Context

Read from two production transcripts (29 July 2026):

- **litit** — 16 messages, 30.7 KB. Opening round alone: `cv_context` 11.4 KB, `cv_get` 6.5 KB.
  Then ten `experience_search` calls, four of which returned the same atoms as one another.
  Zero CV edits; the turn ran out of rounds (`ASSISTANT_MAX_STEPS` unset on prod → 8).
- **getambush** — 23 messages, five searches, zero CV edits, plus a round lost to
  `experience_add` guessing an `employment_id`.

The retrieval those ten calls were asking for is `experience.Store.Retrieve` — a linear scan over
the caller's atoms with skill, stack and word-overlap scoring. No model, no network. The agent was
spending tool rounds on something the server could have handed it in the first result.

## Goals / Non-Goals

**Goals:**
- The agent arrives at its first edit knowing what it can evidence, without paying rounds to find out.
- The opening round stops being the most expensive thing in the conversation.
- A wrong id costs a correction, not a round.

**Non-Goals:**
- A batch search tool. If the context already carries retrieval for every requirement, the ten
  calls do not happen — building a batching mechanism for them would be machinery for a problem
  this change removes.
- Changing the retrieval algorithm itself. Its scoring is not what failed here.
- Touching the public `GET /me/cvs/:id/tailor-context` shape. The additions belong to the agent's
  view of that context; a client that reads the endpoint is unaffected.
- Semantic/embedding retrieval. The word-overlap floor is enough to decide "ask or reframe", and
  an embedding pipeline is a different change with its own cost.

## Decisions

### Enrichment lives in the tool, not in `tailorContext`

`tailorContext(analysis, job)` is shared by the HTTP endpoint and the agent's tool. The evidence
attachment goes in the TOOL — `cvContextTool` — because it is the agent that needs it and the
agent that has an experience store to hand. The endpoint keeps serving what it serves.

Alternative rejected: enriching `tailorContext` itself. It would push a bank dependency into a
handler that has no business holding one, and would put per-requirement evidence into a response
the SPA renders, where nothing reads it.

### The evidence is named, not inlined

Each requirement carries the atoms' ids and claims, bounded — not their full context, metrics and
role blocks. The agent needs enough to decide "I can write this" and the id to cite; the rest it
can fetch with `experience_search` for the one requirement where it matters. A tool result is
replayed into the model's context on every later turn, so what goes in it is paid for repeatedly.

### An empty list, not an absent field

A requirement with nothing in the bank reports `evidence: []`. Omitting the field would read as
"not looked at", and the difference between "looked and found nothing" and "did not look" is
exactly the difference between asking the candidate and staying silent.

### Measured, not assumed: where the 11.4 KB actually went

Breaking the recorded `cv_context` result down by field: `job` 4235 B (the posting, as HTML),
`missing_gap` 2297, `dimensions` 1756, then `gaps` 472, `recommendation` 402, `strengths` 399,
`missing_have` 199.

Rendering the description to markdown saves ~700 B on that posting — real, but a tenth of what
the first guess said. The larger and easier saving is the ~3 KB of dimension comments, strengths,
gaps and recommendation: the agent cannot edit a CV from any of them, and the prompt now forbids
restating them, so carrying them through every subsequent turn buys nothing. The agent's context
drops them; the HTTP endpoint keeps serving them to the page that renders them.

### The description is rendered, not stripped by the model

`get_job` already renders the stored HTML to text before it reaches a model, with a test that says
so. The tailoring context did not — an inconsistency in how the same field reaches the same model
through two tools. Same helper, same treatment.

## Risks / Trade-offs

- **The context result grows a little** → It shrinks overall: ~700 B off the description and ~3 KB
  of narrative sections dropped, against a few hundred bytes of evidence per requirement. Both the
  description and the evidence lists are bounded.
- **Retrieval runs even when the agent would not have searched** → It is a linear pass over tens
  of atoms with no I/O beyond two owner-scoped reads. Cheaper than the round it replaces.
- **A model may now cite an id it never searched for** → Unchanged where it matters: `cv_edit`
  still resolves the id and refuses anything not publishable. The context reports `can_write_cv`
  per atom exactly as search does.
- **The prompt asks for edits earlier, which could mean shallower edits** → The instruction is to
  edit per requirement as it is closed, not to edit faster; the evidence wall is what bounds
  quality, and it is untouched.

## Migration Plan

No schema, no endpoint, no client change. Ship with the code; nothing to backfill and nothing to
roll back beyond the release itself.
