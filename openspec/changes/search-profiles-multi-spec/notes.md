# Implementation notes

## Task 1.1 — root cause of the inactive Create button (systematic-debugging Phase 1)

**Symptom:** on `/my/profiles` the "Create profile" button never enables.

**Mechanism (traced in `web/src/lib/components/SearchProfilesView.svelte`):**
- The button is disabled unless
  `canSubmit = name.trim() !== '' && specialization !== '' && skills.length > 0`.
- Skills can only be added by `toggleSkill`, which is called by the `SearchSelect` pill
  wall **for options that already exist** in `skillOptions` (= `skillDist`, the live skills
  facet distribution + any already-selected skills on edit). There is **no code path that
  adds a typed value**.
- Therefore, if the user's desired skill is not among the canonical dictionary tokens, the
  filter shows "Nothing found", nothing is selectable, `skills.length` stays `0`, and
  `canSubmit` is permanently `false`.

**Evidence:** the live facet endpoint returns 268 canonical skills
(`GET /api/v1/jobs/facets` → `data.facets.skills`), so the dictionary loads in prod — the
dead-end is specifically "skill absent from the dictionary". The backend does **not**
restrict skills to a vocabulary (`normalizeSkills` only lowercases/trims/dedupes), so the
restriction is purely a frontend artifact of the select-only picker.

**Fix (this change):** replace the skills pill wall with a dictionary-backed typeahead
(`RemoteSearchSelect`) that surfaces matching skills as chips, and make specialization a
multi-select. Product decision keeps skills dictionary-only, so the "nothing found" case is
shown explicitly rather than silently dead-ending.

## Verification (task 7.2) — real-Postgres E2E

Brought up the worktree's Postgres on a fresh volume (migration 0029 ran automatically:
`\d search_profiles` shows `specializations text[]`, no `specialization` column, and the
`cardinality BETWEEN 1 AND 5` CHECK) and ran the Go server on host. Round-trip via the API
with a cookie session:
- create `{"specializations":["backend","devops"],"skills":["Go","Docker","go"]}` → 201,
  stored `["backend","devops"]` + normalized `["go","docker"]`.
- rename-only PATCH `{"name":...}` → 200, **specializations/skills unchanged** (empirically
  refutes review #1: a nil `[]string` param encodes as SQL NULL, so COALESCE keeps the
  stored array — same proven pattern as the pre-existing `skills`).
- specs-only PATCH → 200, skills unchanged.
- empty `[]` → 400; six specializations → 400; unknown category → 400.

## Prod migration reminder (task 7.3)

Apply `migrations/0029_search_profiles_specializations.sql` via manual `psql` **before**
rolling the new server binary — the new sqlc queries reference the `specializations`
column. Rollback SQL is captured as a comment in the migration file.
