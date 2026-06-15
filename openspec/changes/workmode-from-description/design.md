## Context

`internal/location` already owns work-mode detection: `detectWorkMode(lower)` does a
substring scan of the location string against `workModeMarkers` (hybrid > remote >
onsite). `jobderive.Derive` resolves `work_mode` as structured (ATS) → parsed
location. The description — where work arrangement is most often stated — is never
read, which is the 85%-of-enriched coverage gap this change closes.

## Goals / Non-Goals

**Goals:**
- A deterministic, high-precision `work_mode` derivation from the description.
- Description is the lowest-priority source: structured → location → description.

**Non-Goals:**
- seniority/category from description; skill-vocabulary expansion; any LLM change.
- Re-reading the description for anything but work mode.

## Decisions

**Decision: a separate, conservative phrase set, not the location markers.** The
short-location markers (`anywhere`, `worldwide`, `distributed`, bare `remote`/
`hybrid`) are safe in a one-line location but produce false positives in long prose
("distributed systems", "hybrid cloud", "remote team", "remote server"). The
description detector uses its own curated, anchored phrases tuned for precision over
recall (`fully remote`, `remote-first`, `work from anywhere`, `hybrid role/working/
model`, `days in the office`, `on-site only`, `must be on-site`, `in-office
position`, …). It emits nothing on a weak/ambiguous signal — the dictionary doctrine
("never guess") is preserved. *Why not reuse `detectWorkMode`:* it would import the
unsafe markers; precision is the whole point.

**Decision: lives in `internal/location` as `WorkModeFromDescription(desc) string`.**
The package already owns the work-mode vocabulary (aligned to `enrich.WorkModeValues`)
and the detection idiom; a one-function addition is cheaper than a new package and
keeps the work-mode logic in one place.

**Decision: wire as a fallback in `jobderive.Derive`.** After the existing
`structured → location` resolution, add `if workMode == "" { workMode =
location.WorkModeFromDescription(in.Description) }`. This is the only call-site
change; `cmd/backfill-derive` re-derives through `jobderive`, so it inherits the new
source with no edit.

## Risks / Trade-offs

- **False positives from noisy prose** → Mitigation: a curated anchored phrase set
  plus explicit negative ("trap") tests (`distributed systems`, `hybrid cloud`,
  `remote server` → empty). Bias to precision; missed signals stay empty (the LLM
  discovery layer still has them raw).
- **Phrase set is necessarily incomplete** → Acceptable: it is a curated dictionary,
  extendable later; partial recall beats the current zero.

## Migration Plan

Ships with `dict-production-facets`' deferred deploy: deploy the app image, run
`cmd/backfill-derive` once (now recovers description-derived work_mode), then one
`reindex`. No schema change, no rollback data concern (re-derivation is idempotent).
