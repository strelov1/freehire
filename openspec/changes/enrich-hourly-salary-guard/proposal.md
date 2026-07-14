## Why

Jobs quoting a fractional hourly rate (e.g. DoorDash's `$26.08–$38.40 / hr`) are
displayed with a salary inflated ×100 — `2 608 – 3 840 $ / hr`. The enrichment
LLM, told the contract wants `salary_min (int)`, strips the decimal point of a
sub-dollar-precision rate (`26.08 → 2608`) instead of rounding it (`→ 26`). A
live spike against the production model confirmed the bug reproduces on the
current prompt (and also mangles `salary_period`), and that a single prompt
guard with a concrete counter-example fixes it deterministically (3/3 runs
returned `26 / 38 / hour`). This corrupts every hourly-rate posting with cents —
a mass format in US retail/logistics.

## What Changes

- Add an explicit instruction to the enrichment system prompt: salary figures
  are **whole units of the currency**; an hourly rate written with cents
  (`$26.08/hr`) MUST be rounded to the nearest whole unit (`26`), and the
  decimal point MUST NEVER be stripped (`26.08` must never become `2608`).
- Bump `enrich.Version` so already-corrupted jobs are re-enriched through the
  corrected prompt (the existing re-enrichment mechanism).

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `ai-enrichment`: the requirement that enrichment is extracted from a job's
  description by an LLM provider gains a constraint — the provider MUST instruct
  the model to represent salary amounts as whole currency units and to preserve
  fractional hourly rates by rounding, never by dropping the decimal point.

## Impact

- `internal/enrich/langchain.go` — the salary line of the system prompt.
- `internal/enrich/enrichment.go` — `enrich.Version` bump (triggers re-enrich of
  existing rows).
- Tests: `internal/enrich/langchain_test.go` (prompt asserts the guard).
- No schema, migration, API, or frontend change. `salary_min`/`salary_max` stay
  `*int`; this change makes the LLM populate them faithfully rather than
  changing their representation.
