## Why

Two real tailoring sessions on production, read from their transcripts:

| Session | Messages | `experience_search` calls | CV edits |
|---|---|---|---|
| litit, 13:01 | 16 (30.7 KB) | 10 | **0** |
| getambush, 11:33 | 23 | 5 | **0** |

Neither changed the CV. The first spent 18 KB of its 30 KB on the opening round alone —
`cv_context` returned **11.4 KB**, `cv_get` 6.5 KB — then made ten searches, four of which
returned the same atoms as each other, and ran out of tool-call rounds before editing anything.

Two things cause that. `cv_context` hands the model the vacancy description as **raw HTML**,
while the neighbouring `get_job` renders the same field to markdown. And it reports the
requirements without saying anything about the bank, so the agent has to ask about each one
separately — even though the retrieval that answers those questions is a local scan over the
candidate's own atoms, with no model call in it.

## What Changes

- `cv_context` renders the vacancy description to markdown and bounds it, the way `get_job`
  already does. The HTTP endpoint of the same name is unaffected — this is the agent's view.
- `cv_context` attaches the bank's own answer to every requirement it reports: the ids and
  claims that retrieval already scores against it. The agent arrives knowing what it can
  evidence and what it must ask about, instead of discovering it one call at a time.
- `experience_add`'s employment error names the valid roles and their ids, so a model that
  guessed can correct itself in the same round rather than spending one on
  `experience_employments`.
- The tailoring prompt tells the agent to edit as it goes and to stop restating the verdict —
  both sessions opened with a long summary of an analysis the candidate has open beside the chat.

## Capabilities

### New Capabilities
<!-- none: this sharpens an existing capability rather than adding one -->

### Modified Capabilities
- `cv-tailoring`: the tailoring context an agent reads now carries retrieved evidence per
  requirement and a markdown description, rather than a raw posting and no bank information.
- `assistant-agent-runtime`: a rejected tool call reports what a valid argument would have been,
  not only that the one sent was wrong — a correction that costs a round is a correction the
  step cap pays for.

## Impact

- **Go:** `internal/handler/cv_tailor.go` (the context shape), `internal/handler/assistant_cv_tools.go`
  (the tool's own enrichment), `internal/handler/assistant_experience_tools.go` (the error),
  `internal/assistant/prompt.go` (the tailoring prompt).
- **No schema change, no new endpoint, no client change.** The wire shape of
  `GET /me/cvs/:id/tailor-context` keeps its current fields; the additions are the agent's.
- **Not affected:** the fit analysis itself, the autopilot run's mechanics, the experience bank's
  provenance rule.
