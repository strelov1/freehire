## Why

Choosing the assistant's model is currently an argument from price lists. A vendor's
per-million figure does not predict what a turn costs here: the transcript is replayed
into context every round, so a model that needs fourteen rounds where another needs five
costs far more than its token price suggests — and #2633 showed the other half of the
same blindness, that 20k tokens a turn were being re-read at full price because the
window slid. Nothing in this repository can answer "which model is better value for the
assistant" with a number, so the answer has been a guess.

The measurement is cheap to build because the two things that look hardest are already
here. `newAutopilotHarness` (`internal/api/handler/assistant_autopilot_integration_test.go`)
already stands a Postgres up and takes the turn model as a parameter, and `cvmatch` and
`atscheck` already score a tailored CV deterministically without an LLM — they are the
numbers the product shows the candidate, so improving them is improving the product
rather than pleasing a judge model.

## What Changes

- A new `llmlive` bake-off that runs the `tailor` autopilot over a fixed case set once
  per candidate model, against a fresh database per model so every candidate sees the
  same world, and reports one row per (model, case).
- Five measurements per run: rounds to `end_turn`, the share of tool calls whose
  arguments failed to decode, input/output tokens, the share of input tokens served from
  the provider's prompt cache, and time to first token.
- Two quality scores per run, both existing and both free: `cvmatch.Compute` over the
  tailored CV against its vacancy, and `atscheck.Compare` between the base and tailored
  reports. No grader model is called: the report also carries each run's tailored CV in
  full, and whether it is any good is read by whoever reads the report.
- `internal/platform/llm` learns to report cached input tokens alongside the input and
  output counts it already reports. It cannot today, which is why #2633 had to measure
  cache behaviour from sixteen days of production logs rather than from the code.
- Candidate model prices are read from the gateway's model catalogue into a checked-in
  file rather than hard-coded, so a price change is visible instead of silent.

## Capabilities

### New Capabilities

- `assistant-model-bakeoff`: running the tailoring autopilot over a fixed case set on
  more than one model and reporting comparable cost and quality figures for each.

### Modified Capabilities

- `llm-observability`: a recorded generation's token usage carries the cached input
  count as well as input, output and total.

## Impact

- **New**: a `//go:build llmlive` bake-off in `internal/api/handler`, its case fixtures
  and profile fixture under `testdata/`, and a checked-in model price table.
- **Modified**: `internal/platform/llm` (usage shape and its Langfuse mapping),
  `newAutopilotHarness` where the bake-off needs a seam it does not yet expose.
- **Unmodified on purpose**: no `cmd/` worker, no production code path, no schema change.
  The bake-off is a test; nothing it does runs for a user.
- **Depends on** `origin/main` at or after #2633 (block-aligned history window) and #2635
  (attribution keeping the entrypoint's timeout). Measuring cache behaviour against the
  sliding window would report the bug rather than the model.
- **Settled, not open**: the cached-token count reaches us already. OpenRouter returns
  `usage.prompt_tokens_details.cached_tokens`, langchaingo v0.1.14 surfaces it as
  `GenerationInfo["PromptCachedTokens"]`, and `UsageFrom` simply does not read that key.
  The limitation that remains is one the library imposes: it writes the key from a
  zero-valued struct, so an unreported count and a genuine cache miss are both `0`. The
  bake-off states that rather than papering over it.
