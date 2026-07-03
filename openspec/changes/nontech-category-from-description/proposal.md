## Why

The AI-enrichment cost gate (shipped Step 1) skips jobs whose derived `category` is
confidently non-technical, but `classify` derives `category` from the **title only**,
so on prod ~71% of open jobs have an empty `category` and fall through the gate into
LLM enrichment. Many of those are plainly non-technical roles whose descriptions state
the role clearly — recovering them deterministically from the description extends the
gate and cuts more LLM cost, at zero schema cost.

## What Changes

- Add a deterministic **non-tech category detector over the job description**:
  `classify.NonTechFromDescription(desc)` returns a confidently non-technical
  `category` (`marketing`/`sales`/`support`/`management`) or empty, using
  intent-anchored role-statement phrases (never bare words), mirroring the existing
  `SeniorityFromDescription`.
- Extend the `category` derivation precedence in `jobderive.Derive` with a third tier:
  structured source → title dictionary → **description non-tech detector**. The lower
  tier only fills a `category` the higher ones left empty; the detector emits nothing
  when unsure (an empty category stays enriched — the safe direction).
- Apply the same description fallback in `cmd/tg-extract` (which derives category via
  `classify.Parse` directly, not through `jobderive`).
- No behavior change to the enqueue gate itself: it already reads `jobs.category`, so a
  newly-populated non-tech category is skipped automatically.

## Capabilities

### New Capabilities

<!-- none -->

### Modified Capabilities

- `deterministic-facets`: the `category` facet, currently title-only, SHALL also be
  derived from the description via a precision-anchored non-tech detector when the
  structured source and title dictionary are both silent. (Today this capability
  states "The category facet is unaffected" by the description tier — that clause is
  what changes.)

## Impact

- Code: `internal/classify/description.go` (new function + phrase tables),
  `internal/jobderive/jobderive.go` (category precedence third tier + stale comment),
  `cmd/tg-extract/store.go` (two derive sites). Unit tests in `internal/classify` and
  `internal/jobderive`.
- No schema/migration changes, no new dependencies. The Step-1 enqueue gate is
  untouched.
- Ops: new ingests gate automatically; existing empty-category jobs need a
  `cmd/backfill-derive` pass (then optionally a one-time delete of non-tech pending
  `enrichment_outbox` rows) to realize savings on the current backlog.
