## MODIFIED Requirements

### Requirement: Five-dimension scored verdict

The analysis payload SHALL contain five dimensions — Title & role alignment, Experience
relevance, Seniority fit, Skills coverage, and Company & role context — each with an integer
score clamped to 0–100, plus a weighted `overall_score`, a `verdict` label drawn from the
controlled set {Strong Fit, Good Fit, Moderate Fit, Weak Fit, Poor Fit}, a `strengths` array,
a `gaps` array, and a single `recommendation` string. All model output MUST be sanitized: scores
clamped, the verdict coerced to the controlled set, and free-text fields trimmed and length/count
bounded before it is persisted or served. The served `overall_score` MUST additionally be clamped
down to the deterministic hard-constraint ceiling (the minimum score-cap over the caller's unmet
blockers) when any blocker is present, so a posting the caller plainly cannot meet can never present
as a strong fit; the `verdict` label is derived from the capped `overall_score`. The ceiling is
recomputed from the current job, résumé, and hard-constraint dictionary each time the analysis is
served — for both a freshly computed and a cached analysis — so it is never stale and needs no cache
stamp of its own.

#### Scenario: Out-of-range or invalid model output

- **WHEN** the LLM returns a dimension score above 100 or a verdict outside the controlled set
- **THEN** the score is clamped to 0–100 and the verdict is derived from `overall_score`, so no out-of-vocabulary value is ever persisted or served

#### Scenario: Overall score is the weighted dimensions

- **WHEN** the five dimension scores are known and the caller has no unmet hard-constraint blockers
- **THEN** `overall_score` equals the deterministic weighted average of the dimensions, computed server-side rather than trusting the model's own overall

#### Scenario: Hard-constraint ceiling caps an over-optimistic score

- **WHEN** the weighted average is 88 but the caller has an unmet certification blocker with score-cap 60
- **THEN** the served `overall_score` is 60 and the `verdict` label is derived from that capped value

#### Scenario: Cached analysis re-caps on read after a dictionary change

- **WHEN** a cached analysis is served after the hard-constraint dictionary changed such that a blocker is now unmet
- **THEN** the ceiling is recomputed on read and the served `overall_score` reflects the current dictionary without the cached row being marked stale

## ADDED Requirements

### Requirement: Hard-constraint blockers ground the prompt chain

The prompt chain SHALL include the deterministic hard-constraint blockers as known, already-established constraints so the model explains and respects them rather than re-deriving degree, years, license, or work-authorization requirements. The served analysis MUST expose the blockers alongside the verdict.

#### Scenario: Blockers passed into the prompt and surfaced

- **WHEN** the fit analysis is computed for a caller with an unmet hard constraint
- **THEN** the prompt carries the blocker as a known constraint and the served analysis exposes it beside the dimensions
