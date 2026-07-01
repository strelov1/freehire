## Why

A search profile today captures exactly **one** specialization, but real people combine
roles (e.g. "Go backend **and** DevOps") — the single-value model can't express that.
Worse, the skills picker is a select-only pill wall over the canonical dictionary: when a
user can't spot the skill they want they select nothing, and because the Create button is
gated on "at least one skill", the form becomes a dead-end that never enables. This change
lets a profile hold several specializations and replaces the skills pill wall with a
typeahead ("search with autosuggestions") so the required inputs are always reachable.

## What Changes

- **BREAKING** A profile's single `specialization` (a string) becomes `specializations`
  (a non-empty, capped set of job categories). The DB column, the sqlc queries, the
  service, the JSON request/response, and the TS types all move from one value to a set.
- The service validates every specialization against the category vocabulary
  (`enrich.CategoryValues`), trims and deduplicates them, requires at least one, and caps
  the set (max 5) — the same shape as the existing skills normalization.
- The profile form's specialization input becomes a multi-select (reusing the existing
  searchable multi-select), and the skills input becomes a dictionary-backed **typeahead**
  (reusing `RemoteSearchSelect`): as the user types, matching canonical skills with their
  job counts are suggested and added as removable chips.
- Root-cause and fix the "Create profile button never enables" dead-end so the form is
  always completable when the required fields are filled.
- A new migration converts existing rows (each current `specialization` becomes a
  single-element `specializations` set); prod applies it manually (the standing
  versioned-migration-runner seam).

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `search-profiles`: the profile entity gains a multi-valued `specializations` set in place
  of the single `specialization`; specialization validation, the create/list/update wire
  shapes, and the management-UI requirement change accordingly.

## Impact

- **DB**: `search_profiles.specialization TEXT` → `specializations TEXT[]` via a new
  migration (`migrations/0029_*`); sqlc regenerated.
- **Backend**: `internal/searchprofile` (validation + Create/Update signatures),
  `internal/db/queries/search_profiles.sql`, `internal/handler/me_profiles.go` (request /
  response), and their tests.
- **Frontend**: `web/src/lib/types.ts`, `web/src/lib/api.ts`,
  `web/src/lib/searchProfiles.svelte.ts`, and
  `web/src/lib/components/SearchProfilesView.svelte`.
- **API consumers**: the `/api/v1/me/profiles` request and response change shape
  (`specialization` → `specializations`); the SPA is the only consumer and is updated in
  lockstep.
