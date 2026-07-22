## Why

Users can list the skills they *want*, but not the technologies they want to
avoid. A user who never wants PHP work has to exclude it by hand in the filter
every time. Letting the profile carry an "excluded skills" list — and having
**Apply my profile** seed those as negative filters — removes that repeated
manual step and makes the profile a truer statement of what the user wants.

## What Changes

- Add an optional `excluded_skills` list to the user profile (stored alongside
  the existing `skills` / `specializations` / `location_preferences`). Empty by
  default; canonical lowercase skill tokens, trimmed and deduplicated like
  `skills`.
- On save, drop any excluded skill that also appears in `skills` — a skill
  cannot be both wanted and avoided; the wanted set wins (silent subtraction,
  no error).
- Extend the **Apply my profile** action so each excluded skill is seeded into
  the `skills` facet's **exclude** set (rendering `?skills_exclude=…` →
  Meili `skills != "X"`). The search/filter/exclude machinery already exists
  end-to-end; only the profile-to-filter seeding is new.
- Profile form gains a "Skills to avoid" input mirroring the existing "Skills"
  typeahead (dictionary-constrained to known skill tags).

No breaking changes: `excluded_skills` is additive and optional; existing
profiles and API clients are unaffected.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `search-profiles`: the profile entity gains an optional `excluded_skills`
  set with its own normalization rule (canonicalized like `skills`, and
  subtracted against `skills` so the two sets never overlap).
- `filter-modal`: the **Apply my profile** action additionally seeds each
  profile excluded skill into the `skills` facet's exclude set.

## Impact

- **Database:** new `user_profiles.excluded_skills text[] NOT NULL DEFAULT '{}'`
  column (standalone migration; manual apply on prod before deploy, per the
  0010 precedent).
- **Backend:** `internal/db/queries/user_profiles.sql` (+ sqlc regen),
  `internal/userprofile` (Profile struct, normalization, repository),
  `internal/handler/me_profile.go` (request/response).
- **Frontend:** `web/src/lib/types.ts` (`UserProfile`), `web/src/lib/api.ts`
  (`saveProfile`), `web/src/lib/profile.svelte.ts`, `web/src/lib/facetModel.ts`
  (`filtersFromProfile`), `web/src/lib/components/ProfileForm.svelte`.
- **Out of scope:** match analysis / résumé verdict do not yet consume excluded
  skills (noted as a future seam).
