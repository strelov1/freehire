## Context

`internal/classify` derives `seniority`/`category` from the **title** via
`containsWord` whole-word matching of EN+RU aliases in priority order. The title is
short and role-focused, so whole-word alias matching is safe there. The description
is long prose where the same aliases are noisy (`senior management`, `lead the
team`, `report to the head of product`), so the title approach cannot be reused.
`jobderive.Derive` resolves `seniority` from the title only; ~64% of enriched jobs
have a description-stated grade the title misses.

## Goals / Non-Goals

**Goals:**
- A deterministic, high-precision `seniority` derivation from the description.
- Description is the lower-priority source: title → description.

**Non-Goals:**
- `category` from the description (its prose signal — tech-stack mentions — is too
  noisy; deferred to its own careful change).
- Years-of-experience → grade banding (the boundaries are a judgment call, not a
  fact; excluded per "never guess").
- Any LLM change; any schema or command change.

## Decisions

**Decision: intent-anchored phrases, not the bare title aliases.** The detector
uses a separate curated set of anchored phrases (`senior-level`, `senior
position/role`, `we are looking for a senior`, `principal engineer`, `staff
engineer`, `entry-level`, `internship`, …). Ambiguous grade words get an intent
anchor: `head of` is matched only as `looking for a head of` / `as head of`, never
bare, so "report to the head of product" does not misfire. Priority follows the
title order: c_level > principal > staff > lead > senior > middle > junior > intern.
It emits `""` on a weak signal — the dictionary doctrine ("never guess") holds.
*Why not reuse `containsWord` with the title aliases:* the bare aliases are unsafe
in prose; precision is the whole point.

**Decision: lives in `internal/classify` as `SeniorityFromDescription(desc) string`.**
The package already owns the seniority vocabulary (aligned to
`enrich.SeniorityValues`); a one-function addition keeps the grade logic in one
place. The phrase set goes in a new file (`description.go`) to keep `classify.go`
focused.

**Decision: wire as a fallback in `jobderive.Derive`.** After
`class := classify.Parse(in.Title)`, add `seniority := class.Seniority; if
seniority == "" { seniority = classify.SeniorityFromDescription(in.Description) }`,
and return that `seniority`. `class.Category` is returned unchanged.
`cmd/backfill-derive` inherits the new source with no edit.

## Risks / Trade-offs

- **False positives from noisy prose** → Mitigation: intent-anchored phrase set plus
  explicit negative ("trap") tests (`senior management`, `lead the team`, `junior
  colleagues`, `our staff`, `principal component analysis`, `report to the head of
  product` → empty). Bias to precision; missed grades stay empty.
- **Low recall vs years-banding** → Accepted: explicit-phrase recall is lower, but a
  defensible grade beats a guessed one; the LLM discovery layer still has the rest raw.

## Migration Plan

Ships with the deferred dict-only deploy: deploy the app image, run
`cmd/backfill-derive` once (now recovers description-derived seniority), then one
`reindex`. No schema change; re-derivation is idempotent.
