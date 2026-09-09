## Context

Model choice for the assistant is currently argued from vendor price lists. That number
does not predict a turn's cost here, for two reasons this repository already documents.
The transcript is replayed into context every round, so cost grows with rounds and not
just with tokens; and #2633 measured on production that 20k tokens of that replay were
being re-read at full price on every request because the history window slid by one
message, defeating the provider's prefix cache. The first cost driver is a property of
the model (how many rounds it needs), the second is a property of the provider (whether
it caches at all). Neither appears on a price page.

Two pieces of what a measurement needs already exist:

- `newAutopilotHarness` (`internal/api/handler/assistant_autopilot_integration_test.go`)
  stands a Postgres via testcontainers, wires the handlers and routes, and takes the turn
  model as a parameter. Today it is handed a scripted stand-in.
- `cvmatch.Compute` and `atscheck.Score`/`Compare` score a tailored CV against its
  vacancy deterministically and without a model call. They are what the tailoring
  workspace shows the candidate.

What is missing is the cached-token count. `llm.UsageFrom` (`internal/platform/llm/llm.go`)
reads `PromptTokens`, `CompletionTokens` and `TotalTokens` out of langchaingo's
`GenerationInfo` and stops there.

## Goals / Non-Goals

**Goals:**

- One command produces a comparable cost-and-quality row per (candidate model, case) for
  the tailoring autopilot — the assistant's most expensive turn shape.
- Quality is ranked by the product's own deterministic scores, and the tailored CVs
  themselves are put in front of a reader rather than in front of a grader model.
- The cached-token count becomes readable from code, so a cache regression is visible
  without going back to production logs.

**Non-Goals:**

- No `cmd/` worker. Nothing here runs on a schedule or for a user.
- No production behaviour change. The only non-test edit is additive reporting.
- No routing, weighting or fallback logic. Deciding what to do with the answer is a
  separate change.
- No coverage of the other presets. `chat`, `interview` and `browse` are cheaper and
  differently shaped; the autopilot is where the money is.

## Decisions

### An `llmlive` test, not a worker

The harness that stands the database, wires the handlers and accepts a turn model already
exists as a test helper. Writing a `cmd/assistant-eval` would rebuild all of it to reach
the same seam, and would additionally need its own database lifecycle. The repository
already carries a `llmlive` build tag for exactly this class of thing — a test that calls
a real model — and `go test ./...` does not compile it.

*Alternative considered:* a `cmd/` worker producing a JSON report. Rejected as a rebuild
of an existing harness. If the bake-off later needs to run somewhere a test cannot, the
scoring and reporting are pure functions and lift out unchanged.

### A fresh database per model, not per run

Seeding once and running every model against it would let the first model's CV edits
become the second model's starting point. Per-model isolation is what makes the rows
comparable at all. Per-*run* isolation would be stricter still but costs a container per
case for no gain, since cases within one model's pass are independent by construction —
each binds its own CV copy and its own vacancy.

### Only the turn model is swapped

`newAutopilotHarness` takes both a turn model and a fit model. Swapping both would
measure two models at once and attribute the result to one. The fit model stays pinned to
whatever the harness uses today.

### The bake-off measures; the reading agent judges

The bake-off produces two kinds of quality signal and mixes neither into the other.

`cvmatch` and `atscheck` are computed for every run. They are free, pure, identical run
to run, and they are the numbers the candidate already sees in the workspace — a model
that raises them has improved the product rather than impressed a grader. They are what
the report ranks on.

The read of whether a tailored CV is actually *good* is done by the agent reading the
report, over every run and not only over ties. The bake-off therefore emits each run's
tailored CV text in full beside its scores and its vacancy, and stops there: it renders
no verdict of its own and calls no grader.

This is not a compromise for lack of a judge model — it is better than one here. It costs
nothing, adds no model to configure and no grader prompt to validate, and the reader has
this repository, the change's specs and the provenance rules in front of it. A grader
called over HTTP has a rubric and one document. It would also be the third model in a
measurement whose whole point is to attribute a difference to the first.

What this gives up is an unattended verdict: a bake-off run ends with a table and a stack
of CVs, not a winner. That is the honest shape of the question — "which of these CVs is
better" was never a number, and the two that *are* numbers are already in the table.

*Alternative considered:* an LLM judge, primary or as a tie-break. Rejected: a paid
dependency and a second model to validate, producing a noisier answer than a reader with
the repository open.

*Alternative considered:* mechanical measurements only, no quality read. Rejected: the
cheapest and fastest model is the one that answers "I cannot help with that" in a single
round, and a cost-only ranking would crown it.

### Cached tokens: the wire carries it, the mapping drops it

Verified against the pinned dependency rather than assumed:

- OpenRouter returns `usage.prompt_tokens_details.cached_tokens` — its own recorded
  fixtures in langchaingo carry the field.
- langchaingo v0.1.14 already surfaces it as `GenerationInfo["PromptCachedTokens"]`
  (`llms/openai/openaillm.go`).
- `llm.UsageFrom` reads three keys and not that one.

So the change is one key in one function plus a field on `Usage` and its Langfuse
mapping. No dependency bump, no patched fork, no raw-response plumbing.

**One honest limitation, and it shapes a requirement.** langchaingo writes the key
unconditionally from a zero-valued struct, so a provider that reports no cached count and
a request that genuinely hit nothing both arrive as `0`. The transport cannot tell them
apart, and neither can we. The bake-off therefore does not claim to: it reports the
observed count, and labels a model whose every round reports zero across a prefix that
should have been warm as **no cache observed** — an inference drawn over a whole run,
stated as an inference, rather than a per-call measurement it cannot make.

### Prices come from the catalogue into a checked-in file

A hard-coded price is a number that goes wrong silently and is discovered on an invoice.
The table is generated from the gateway's model catalogue and committed with the date it
was captured, which the report repeats. A model missing from the table is reported with
its measurements and without a cost — never with a cost of zero, which would rank it
first.

### The profile fixture is captured once

The case set is the user's real CV and experience bank against real vacancies, because a
synthetic CV would exercise the evidence gate differently from a real one. It is exported
from production once into `testdata/` and read offline thereafter, so the bake-off needs
no production credentials to run.

## Risks / Trade-offs

- **The zero-versus-absent ambiguity in cached tokens** → Named above and handled by
  reporting an inference at run scope rather than a false measurement at call scope. The
  alternative — reading the raw HTTP response beneath langchaingo — buys a distinction
  worth less than the transport layer it would add.
- **A tailored CV read out of context reads as fine** → The report carries each CV beside
  the vacancy it was written for and the requirements the run was walking, because the
  question is never "is this a good CV" but "is this a good CV for this posting".
- **The case set is one candidate's CV** → A model that suits this profile may not suit
  every profile. Accepted for a first measurement, and stated in the report so no one
  reads it as a general ranking. Widening the case set is additive.
- **A candidate model may not support tools or structured outputs at all** → It fails
  fast on the first round and is reported as failed rather than as expensive. This is
  desirable: it is the same failure a deploy would have produced.
- **testcontainers needs Docker** → The same requirement the existing integration suite
  already carries. Nothing new.

## Migration Plan

No deploy and no rollback. The only production-reachable edit is an additional field on
`llm.Usage`, populated where the transport already supplies it and unset otherwise; every
existing reader is unaffected. Everything else lives behind `//go:build llmlive`.

Work happens in a worktree off `origin/main` at or after #2633 and #2635. Measuring cache
behaviour against the sliding window would report the fixed bug rather than the model.

## Open Questions

- How close do two `cvmatch` scores have to be before the ranking stops meaning anything?
  Resolved by reading the first report, not before it — and until then the report simply
  prints both numbers rather than declaring a winner.
- How many cases make a stable ranking? Start with a handful of real vacancies spanning
  different seniorities and categories, and widen if the ranking proves unstable between
  runs of the same model.
