## Why

Moderators reviewing `/moderation?tab=reports` on prod found a repeated pattern: candidates
report a "remote" posting as region-restricted after applying from outside the eligible
country (job_reports 29-34, all Greenhouse-sourced, filed 2026-09-10/11). In every case
`jobs.location` is a bare `"Remote"`/`"remote"`/`"REMOTE"` string with no country, and the
restriction is stated only in the job's `title` (e.g. `"... [Remote-US]"`) or in description
prose the current parser doesn't recognize (`"a fully remote role within New Zealand,
Australia East Coast..."`). Two implementation gaps cause this: `title` never reaches
geography derivation at all, and `EligibilityFromDescription` only matches citizenship/
work-authorization phrasing, not "role/based within `<country>`" restriction statements.

Separately, `internal/dict/location/location.go:184-186` maps a bare `Remote` with no
geography token to `regions=[global]`. This directly **contradicts the already-written**
`job-geography` spec (`### Scenario: A bare remote marker yields no geography` — "both
countries and regions are empty"), and the `remote-region-filters` capability already gives
search a proper `regions=none` ("Not specified") bucket for exactly this case, so the
`global` default is not filling a real gap — it is emitting a false, overclaiming signal for
postings the parser has no actual information about, which is what let these six postings
mislead international candidates into applying.

## What Changes

- Fix `location.go`'s bare-remote-to-`global` default so it matches the existing spec: a
  bare `Remote` with no geography token yields empty `regions`/`countries`, not `global`.
- Feed the job `title` into geography derivation (`jobderive.Derive`), so an ATS-convention
  restriction suffix (`"[Remote-US]"`, `"(Location - Australia or New Zealand)"`) is read the
  same way `location`/`description` already are.
- Broaden `EligibilityFromDescription`'s phrase set beyond citizenship/work-authorization
  wording to also match explicit "role/based within `<country/region>`" restriction prose.
- Reconcile `openspec/specs/job-geography/spec.md` with the corrected behavior by adding the
  new title/description-restriction requirements (the empty-geography-for-bare-remote
  requirement already matches the fix and needs no wording change).
- Add a one-off backfill (following the precedent of `cmd/backfill-remote-perk-false-positive`
  for the same class of over-tagging bug) that re-derives `regions`/`countries`/`work_mode`
  for already-stored jobs currently holding the incorrect `global` default, since `regions` is
  not part of `content_hash` and a plain reindex would never reach existing rows.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `job-geography`: geography derivation additionally reads the job `title` for restriction
  markers and recognizes a broader class of description restriction phrasing; a bare `Remote`
  location with no geography signal (from location, title, or description) yields empty
  `regions`/`countries`, never `global` — bringing the implementation into line with the
  requirement already on record.

## Impact

- `internal/dict/location/location.go` — remove the bare-remote→`global` default.
- `internal/dict/location/eligibility.go` — broaden the restriction phrase set; extend the
  matcher to also scan a supplied title.
- `internal/job/jobderive/jobderive.go` — pass `Title` into the geography derivation call.
- `openspec/specs/job-geography/spec.md` — add the title-input and broadened-description
  requirements.
- New one-off `cmd/backfill-remote-region-restriction` (or equivalent) — re-derives geography
  for jobs currently holding the incorrect `global` default; needs `DATABASE_URL`,
  `MEILI_URL`, `MEILI_MASTER_KEY` (candidates sourced from Meilisearch, same shape as
  `backfill-clearance`); must be followed by a full `make reindex` (regions is not part of
  `content_hash`).
- No API contract change: `regions`/`countries`/`work_mode` remain the same fields with the
  same vocabulary — only which jobs land in `global` vs. empty (`regions=none`) changes.
