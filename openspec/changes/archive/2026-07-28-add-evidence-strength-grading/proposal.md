## Why

Stage 1 of the fit chain classifies each vacancy requirement as `covered` / `synonym-only` / `missing-have` / `missing-gap`, but `covered` is binary: a CV that merely *lists* "Kubernetes" reads identically to one that *shipped* Kubernetes services with a measured outcome. Research on résumé–job matching is consistent that **evidence should outrank keyword presence** — a bare keyword is not the same signal as a metric-backed accomplishment. Without a strength grade the Stage-3 skeptic and the served verdict cannot tell a genuine, defensible match from an ATS keyword that happens to appear, so `skills_coverage` and the `strengths` list over-credit thin evidence.

## What Changes

- Add an `evidence_strength` grade to each Stage-1 requirement — `metric` (accomplishment with a number/scale/outcome) > `scope` (breadth: teams, systems, regions) > `responsibility` (clear ownership with tools/methods) > `keyword` (the term is present but the surrounding evidence is a bare mention or duty-only). Graded only for the two positive statuses (`covered`, `synonym-only`); the two `missing-*` statuses carry no strength.
- Stage-1 prompt asks the model to grade the evidence it cites; the wire `Requirement` gains `evidence_strength`; `sanitizeRequirements` coerces it to the controlled vocabulary (unknown/absent → `keyword` for positive statuses, empty for missing ones) — same "never persist an out-of-vocabulary value" invariant as the existing status field.
- Stage-3 adversarial audit is instructed to treat a `keyword`-strength `covered` match on a **required** requirement as weak support — it must not by itself sustain a high `skills_coverage` score (extends the `synonym-only` demotion just added).
- The requirement-match table surfaces the strength so the served explanation is honest ("you *shipped* X" vs "you *list* X"); the match page renders it as a small per-requirement cue.
- No new endpoint, no DB migration, no enrichment/reindex: `evidence_strength` is model-derived at analysis time and lives in the existing cached analysis payload. The cache self-heals on the next recompute (CV/job/model stamp unchanged means the field simply appears on the next run).

## Capabilities

### New Capabilities
<!-- none — this extends the existing fit-analysis capability -->

### Modified Capabilities
- `job-fit-analysis`: the Stage-1 requirement-match table gains a graded `evidence_strength` per positive requirement, and the Stage-3 audit uses it to demote keyword-only matches on required requirements.

## Impact

- **Wire contract:** `matchanalysis.Requirement` gains `evidence_strength string`; regenerated to TS via `cmd/gen-contracts`.
- **Touched code:** `internal/matchanalysis` — Stage-1 prompt (`analyzer.go`), the `Requirement` shape + controlled vocabulary + `sanitizeRequirements` (`matchanalysis.go`), the Stage-3 prompt (`analyzer.go`), and the requirement rendering into the prompt (`writeRequirements`). Frontend: the requirement row in `web/src/routes/match/[slug]/` shows the strength cue.
- **No migration, no reindex, no enrichment change:** additive to the in-memory/cached analysis payload; degrades gracefully — an old cached analysis without the field reads as `keyword`/empty and refreshes on recompute.
