## Context

The codebase already has a near-identical per-user, owner-scoped, named-entity feature: **saved searches** (`internal/savedsearch` service + `internal/handler/me_searches.go` + `saved_searches` table + `/api/v1/me/searches`). Search profiles are the same architectural shape — a named, user-owned row with validation, a per-user cap, and cookie-only CRUD — differing only in the columns they carry (`specialization` + `skills` instead of a `query` string).

This change deliberately implements only the **entity and its management**. How a profile is later consumed (match scoring, ranked feeds, badges, Telegram) is out of scope and intentionally absent, so this slice ships independently.

Relevant existing facts:

- Next migration number is `0028`.
- The category vocabulary is `enrich.CategoryValues` (`internal/enrich/enrichment.go`).
- Skills across the app are canonical lowercase tokens; the *closed* canonical set is not enumerated in code — the web skills facet is **dynamic**, driven by live Meilisearch facet values. The existing skills filter trusts the picker rather than validating against a fixed list.
- sqlc is the only DB layer; queries live in `internal/db/queries/*.sql`, regenerated via `make sqlc`.

## Goals / Non-Goals

**Goals:**

- A `search_profiles` table and generated sqlc access.
- An `internal/searchprofile` service mirroring `internal/savedsearch` (validation, per-user cap, sentinel errors).
- Cookie-only CRUD handlers under `/api/v1/me/profiles`.
- A web view to create, list, rename, edit, and delete profiles, reusing existing category and skill selectors.

**Non-Goals:**

- Match scoring, per-job match badges, ranked "my matches" feed.
- Telegram notifications.
- A `seniority`/level field on the profile (deferred).
- Any change to ingest, enrichment, search ranking, or Meilisearch indexing.

## Decisions

**1. New entity, not an extension of `saved_searches`.**
A saved search stores a serialized filter query string; a profile stores structured `specialization` + `skills`. Overloading one table with a polymorphic payload would muddy both. A dedicated table keeps each entity's invariants clear and matches the "separate responsibilities" convention. *Alternative considered:* add columns to `saved_searches` — rejected: the two have different validation and different futures.

**2. Mirror the `savedsearch` package structure exactly.**
`searchprofile.Service` + `Repository` interface + sentinel errors (`ErrInvalidName`, `ErrDuplicateName`, `ErrCapExceeded`, `ErrNotFound`, plus new `ErrInvalidSpecialization`, `ErrEmptySkills`). Handlers stay thin and lean on the central `ErrorHandler`. Reuses the proven cap (50) and name bounds (1–100 runes). This is the lowest-risk path and keeps the codebase consistent.

**3. Storage shape.** `specialization TEXT NOT NULL`, `skills TEXT[] NOT NULL`. A DB `CHECK` enforces `cardinality(skills) > 0` and the name length bound, as `saved_searches` does for its name. `UNIQUE (user_id, name)`. `specialization` is validated in the service against `enrich.CategoryValues` (a DB-level CHECK against an enum would couple the schema to a Go vocabulary that evolves — keep the vocabulary check in Go, as the rest of the app does).

**4. Skills are normalized, not dictionary-validated.**
On write, each skill is lowercased, trimmed, and deduplicated; the set must be non-empty. We do **not** reject unknown skills, because there is no closed canonical skill set in code — the canonical set is the dynamic facet. This mirrors how the existing skills filter trusts the picker. *Alternative considered:* validate against `skilltag`'s alias dictionary — rejected: it's an alias→canonical map for parsing free text, not an authoritative membership oracle, and would reject legitimate facet values it doesn't know.

**5. Response shape.** `{"data": {id, name, specialization, skills, created_at, updated_at}}` for single items and `{"data": [...]}` for the list, `user_id` omitted — consistent with the project's envelope and with `me_searches.go`.

## Risks / Trade-offs

- **A profile that nothing consumes has no immediate user payoff** → Accepted: this is a deliberate foundational slice; consumption is a fast follow. The management UI still lets users build their profile now.
- **Skills aren't validated against a closed set, so typos/non-canonical tokens can be stored** → Mitigated by sourcing skills from the same dynamic facet picker the filters use; normalization (lowercase/trim/dedupe) catches the common noise. Acceptable for an MVP and symmetric with the existing skills filter.
- **`specialization` vocabulary lives in Go, not the DB** → Consistent with the rest of the app (facets/enums are validated in Go); the table stays decoupled from a moving vocabulary.

## Migration Plan

- Add `migrations/0028_search_profiles.sql` (table + indexes + CHECKs). Migrations apply via Postgres initdb on a fresh volume; on a persistent DB this is applied manually (per the project's known migration-runner seam) — note it in the rollout.
- `make sqlc` regenerates `internal/db` from the new queries.
- No data backfill, no reindex (Meilisearch untouched).
- Rollback: drop the table; no other surface depends on it.

## Open Questions

None. Seniority/level and all consumption paths are explicitly deferred to separate changes.
