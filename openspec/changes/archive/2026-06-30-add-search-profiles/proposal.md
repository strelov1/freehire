## Why

Job seekers want to describe themselves once — their specialization and skill set — and reuse that as the basis for finding relevant work. Today a user can save an ad-hoc search filter, but there is no first-class notion of "my profile" (what I do and what I know). This change introduces that entity. Consumption of the profile (match scoring, ranked feeds, notifications) is deliberately out of scope here; this is the foundational data + management slice it builds on.

## What Changes

- Add a user-owned **search profile** entity: a named record of a `specialization` (one job category) and a non-empty set of `skills`.
- A user may keep multiple profiles (e.g. "Go backend", "DevOps"), each with a unique name.
- Add cookie-authenticated CRUD endpoints under `/api/v1/me/profiles` (list, create, update, delete), mirroring the existing saved-searches surface.
- Add a web page for managing profiles: create a profile, pick specialization and skills with the existing facet selectors, rename, delete.
- **Out of scope (deferred to later changes):** match scoring, per-job match badges, a ranked "my matches" feed, Telegram notifications, and a seniority/level field on the profile.

## Capabilities

### New Capabilities

- `search-profiles`: A user-owned profile (name + specialization + skills) with create/list/update/delete, cookie-only and owner-scoped.

### Modified Capabilities
<!-- None. No existing spec's requirements change. -->

## Impact

- **Database:** new `search_profiles` table (migration); regenerated sqlc queries.
- **API:** new `GET/POST/PATCH/DELETE /api/v1/me/profiles` handlers wired in `internal/handler`; new `internal/searchprofile` service mirroring `internal/savedsearch`.
- **Validation:** reuses `enrich.CategoryValues` for specialization; skills normalized (lowercase/trim/dedupe) against the existing canonical skill conventions.
- **Web:** new profile-management view under `web/`, reusing existing category and skill facet selectors.
- No changes to ingest, enrichment, search ranking, or the Telegram pipeline.
