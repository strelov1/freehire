## Context

A search profile (`search_profiles` table, `internal/searchprofile` service,
`internal/handler/me_profiles.go`, and the SPA's `SearchProfilesView.svelte`) stores one
`specialization TEXT` and a `skills TEXT[]`. The skills column is already free-text at the
DB/service layer (only lowercased, trimmed, deduplicated — no vocabulary check); the
frontend, however, restricts skill entry to a select-only pill wall built from the live
skills facet distribution. That mismatch plus the "at least one skill" Create gate is what
makes the form appear un-submittable when a user can't spot their skill.

This change turns the single specialization into a capped, non-empty set and replaces the
skills pill wall with a dictionary-backed typeahead, both reusing components the SPA
already ships.

## Goals / Non-Goals

**Goals:**
- A profile holds 1..5 specializations, each a valid `enrich.CategoryValues` category.
- Skills entry is a typeahead over the canonical skill dictionary with job counts and
  removable chips (dictionary-only — no free-text additions).
- The Create/Save control is enabled exactly when name + ≥1 specialization + ≥1 skill are
  present, with no silent dead-end.
- Existing profiles migrate losslessly (single specialization → single-element set).

**Non-Goals:**
- How profiles are consumed (matching, feeds, notifications) — still out of scope.
- Making skills open-vocabulary (the user chose dictionary-only).
- A versioned migration runner (the standing seam; prod applies SQL manually).

## Decisions

### 1. Data model: rename `specialization` → `specializations TEXT[]`, don't add a join table
A `TEXT[]` mirrors the existing `skills TEXT[]` exactly (same normalization, same CHECK
backstop, same sqlc mapping to `[]string`), so the service, queries, and handler stay
symmetric and simple. A join table would add ordering/uniqueness machinery for no gain at
this scale (≤5 values per profile, no querying by specialization yet).
- *Alternative — keep `specialization` + add `extra_specializations`*: rejected, awkward
  and asymmetric.
- *Alternative — join table `search_profile_specializations`*: rejected as over-engineering
  for the current need (the deterministic-facet columns elsewhere use `TEXT[]` too).

### 2. New migration `0029`, not an edit to `0028`
Migrations apply only on fresh volume init and `0028` already shipped, so editing it would
desync any persistent DB. `0029` ALTERs the table: add `specializations TEXT[]`, backfill
`ARRAY[specialization]`, add the `cardinality BETWEEN 1 AND 5` CHECK, drop the old column.
Prod applies it manually (per the deploy convention).

### 3. Service validation mirrors `normalizeSkills`
Add `normalizeSpecializations([]string) ([]string, error)`: trim each, drop blanks, reject
any value outside `enrich.CategoryValues`, dedupe preserving first-seen order, require
≥1 and ≤5. `Create`/`Update` signatures change `specialization string` →
`specializations []string`; `Update` treats a nil slice as "unchanged" and a
provided-but-empty slice as an error (same as skills). Sentinels: replace
`ErrInvalidSpecialization` semantics and add `ErrEmptySpecializations` /
`ErrTooManySpecializations`, all mapped to `400`.

### 4. Frontend reuse: `SearchSelect` (multi) for specializations, `RemoteSearchSelect` for skills
`SearchSelect` is already a searchable multi-select — the specialization input becomes an
array + toggle with a client-side cap of 5. `RemoteSearchSelect` is already the
typeahead-with-chips pattern; its `search(query)` is backed by a **local** filter over the
already-loaded skill distribution (wrapped in a resolved promise), so there's no new
endpoint and the dictionary-only rule is enforced by construction (only listed options are
addable). This directly fixes the dead-end: typing surfaces matches and adds chips.

### 5. Wire shape is a hard rename (BREAKING), updated in lockstep
`specialization: string` → `specializations: string[]` across `types.ts`, `api.ts`,
`searchProfiles.svelte.ts`, and the view. The SPA is the only consumer and ships together,
so no compatibility shim is warranted.

## Risks / Trade-offs

- **Prod migration is manual** → document `0029` in the change and call it out at finish;
  until applied, the new code's queries reference `specializations` and would error against
  the old column. Deploy migration before the new binary.
- **Breaking wire change** → the only client is the bundled SPA, updated in the same
  change; no external API consumers exist for `/me/profiles`.
- **Dictionary-only skills keep a narrow dead-end** (a skill absent from the 268-token
  dictionary still can't be added) → accepted per product decision; mitigated by a clear
  "nothing found" hint so it's explicit, not silent.
- **Skill distribution fails to load** → the typeahead shows no suggestions; on edit the
  profile's existing skills still render as chips (seeded into the picker), so they remain
  removable.

## Migration Plan

1. Ship `migrations/0029_search_profiles_specializations.sql`; run `make sqlc`.
2. Deploy: apply `0029` to prod via manual `psql` **before** rolling the new server binary
   (the new queries require the new column).
3. Rollback: `0029` is additive-then-destructive; a rollback needs the inverse SQL
   (re-add `specialization TEXT`, backfill `specializations[1]`, drop `specializations`).
   Capture it in the task notes.

## Open Questions

- None. Specialization cap fixed at 5; skills remain dictionary-only per the product
  decisions taken during proposal.
