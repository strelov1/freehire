## Context

`internal/enrich/langchain.go:buildSystemPrompt` currently asks the LLM for a
subset of the `Enrichment` contract; `internal/enrich/schema.go:unaskedFields`
mirrors that subset into the strict JSON request schema. Both were narrowed by
`enrich-prompt-trim` (#659), which correctly identified that `work_mode`,
`seniority`, `category`, `skills`, `employment_type`, `education_level`,
`english_level`, `posting_language`, and `experience_years_min` are all served
to readers from deterministic dictionaries (`internal/jobderive`), so the LLM's
copies were pure discovery material with no serving path — and paid-for output
tokens regardless.

The `Enrichment.Skills []string` field (`internal/enrich/enrichment.go`) was
never removed from the struct; it simply reads back empty on every enrichment
since #659, because nothing asks the model to populate it.

## Goals / Non-Goals

**Goals:**
- Resume collecting the LLM's raw `skills` guess into the existing
  `Enrichment.Skills` field, going forward, as a discovery signal for future
  `internal/skilltag` dictionary expansion.
- Touch only what's needed to re-request and re-accept `skills` — no change to
  how `skills` is served today (`internal/skilltag` dict-based tagging is
  untouched and remains the sole served source).

**Non-Goals:**
- Reverting any of the other eight fields `enrich-prompt-trim` dropped. Their
  taxonomies are closed and slow-changing (grade levels, employment types,
  education levels, …); discovery has near-zero marginal value there, so
  re-requesting them would reintroduce the exact waste #659 fixed for no
  offsetting benefit.
- Building a pipeline that aggregates/dedupes raw `skills` values into
  `internal/skilltag` dictionary candidates. This change restarts raw capture
  only; someone will eventually need to read `jobs.enrichment->>'skills'`
  across the catalogue and decide what's missing from the dictionary, but that
  read/aggregation step is separate follow-up work, not part of this change.
- A new field, table, or JSON key. `Enrichment.Skills` / `"skills"` already
  exists in the contract and is already exempted from validation (see
  `enrichment.go`'s existing "non-enum countries/skills… deliberately NOT
  validated" comment) — reusing it is strictly smaller than adding
  `raw_skills` alongside it, and avoids two similarly-named fields.

## Decisions

- **Prompt-only + schema-only edit.** `buildSystemPrompt` regains the
  `skills (array of lowercase tokens, e.g. go, postgresql)` line in "Other
  keys", worded exactly as it was pre-#659. `unaskedFields` in `schema.go`
  drops `"skills"`. `Validate`/`Sanitize` need NO code change — `skills` was
  already carved out of `servedScalarEnums` and already documented as
  unvalidated discovery material; that behavior is exactly what's needed once
  the model starts populating it again.
- **Comment cleanup, not behavior change.** Three comments currently assert
  `skills` is dict-only/unrequested (`schema.go`'s `unaskedFields` doc comment,
  `enrichment.go`'s `servedScalarEnums`/`Validate` doc comments). They get
  reworded to reflect that `skills` is a requested discovery facet again,
  alongside `countries`/`regions` — no logic in those functions changes.
- **No version bump, no backfill.** Consistent with the forward-only
  convention `enrich-prompt-trim` itself established for this exact class of
  change: existing enrichment payloads keep an empty `skills` array; only new
  or re-run enrichments populate it.

## Risks / Trade-offs

- **Renewed per-call token cost.** Every enrichment call now pays for a
  `skills` array in the output again. Accepted: it's one field of the nine
  #659 removed, and it's the one field in that set with an active, stated
  future use (dictionary discovery) rather than none.
- **No consumer yet.** Until a follow-up change reads and aggregates the raw
  `skills` values, this collects data nobody looks at — same shape of
  criticism #659 leveled at the original nine-field discovery capture.
  Accepted deliberately: the user wants collection to resume now so the
  backlog of raw signal starts accumulating (forward-only, no backfill) ahead
  of the aggregation work, rather than losing weeks of signal waiting for the
  aggregation design to land first.
- **Budget-model prompt sensitivity.** Same caution `enrich-prompt-trim` noted
  in reverse: adding a key back changes prompt length/order. Tests must assert
  the full expected key set (previously-retained keys still present, `skills`
  now present, the other eight still absent) rather than just checking
  `skills` in isolation.
