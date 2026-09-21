## Why

`search.CategoryUnresolved` keeps a job out of the Meilisearch index when its
`category` is empty and `is_tech` is not confidently true. That rule is right in
intent — it stops a broad ATS crawl's undifferentiated bulk from diluting the
index — but the classify dictionary has no aliases for the administrative /
virtual-assistant / immigration family, so **3,554 of 4,440 live postings in that
family (80%) are absent from search altogether**. A candidate who asks us for
remote admin, support, VA or immigration-assistant work cannot find what the
catalogue already holds. One such request arrived by email on 2026-09-18 and is
what prompted this change.

The gap is spelling variants, not taxonomy: `administration`, `legal`,
`support`, `operations` already exist in `vocab.NonTechCategories`, and
`administrative assistant` / `executive assistant` / `receptionist` already
resolve. `admin assistant` (319 live), `virtual assistant` (105) and the whole
immigration family do not.

## What Changes

- Add title aliases to the classify dictionary's ADMINISTRATION block: `admin
  assistant`, `administrative coordinator`, `administrative specialist`, `front
  desk`, `virtual assistant`.
- Add the immigration family to the LEGAL block: `immigration paralegal`,
  `immigration specialist`, `immigration assistant`, `immigration consultant`,
  `immigration case manager`.
- Establish as an explicit invariant that the **bare alias `assistant` is never
  added**. It is what makes this change small: `maintenance assistant`,
  `assistant controller`, `assistant superintendent`, `clinic assistant` and the
  rest of the grade-word tail only misfire against a bare alias, so they need no
  qualifier entries and no `categoryNone` sentinels. They stay uncategorised,
  exactly as today.
- Add canonical skills for the segment's tooling: `freshdesk`, `calendly`,
  `clio`, `uscis`, `i-129`, `i-130` — each with its slug, its label, and its
  `descriptions.tsv` sentence in the same commit.
- Add remote phrases to `descriptionWorkModePhrases` in the register these
  postings actually use: `work from home`, `work-from-home`, `home-based`, `home
  based`, `telecommute`, `virtual position`. `work from home` is evaluated as a
  `travelPerkPhrases` guard candidate, the same treatment `work from anywhere`
  already carries.
- Re-derive with `cmd/backfill-derive` plus a full `make reindex`, which also
  clears the outstanding 2026-09-15 debt (#2847/#2849, ~6,300 postings reading
  `is_tech = true` against the current dictionaries).

Not breaking: every change adds resolution where there was none. No existing
alias is removed or re-pointed.

Deferred deliberately: `personal assistant` (755 live uncovered). Where the
phrase resolves today it has already split across `management` (69),
`operations` (29), `administration` (23) and `support` (21), and in the UK it
also names a social-care worker. `classify/AGENTS.md` requires sampling a phrase
against live titles before admitting it; that sampling is its own change.

## Capabilities

### New Capabilities

<!-- None. Every affected capability already exists; this change extends their
     coverage contracts rather than introducing a new one. -->

### Modified Capabilities

- `role-category-alias-coverage`: adds the administrative/VA and immigration
  clusters to the alias coverage contract, and records the bare-`assistant`
  exclusion alongside the existing bare-"safe"/bare-"compliance" word traps.
- `skill-tag-matching`: adds the support/VA/legal tooling canonicals to the
  dictionary's required coverage.
- `deterministic-facets`: extends the description-derived work-mode phrase set
  with the home-working register.

## Impact

- `internal/dict/classify/dictionaries.go` — alias table (ADMINISTRATION, LEGAL).
- `internal/dict/skilltag/` — `dictionaries.go`, `labels.go`, `descriptions.tsv`.
- `internal/dict/location/workmode.go` — `descriptionWorkModePhrases`, possibly
  `travelPerkPhrases`.
- Operational: one `cmd/backfill-derive` run (~15h; hold `BACKFILL_CONCURRENCY`
  at 2-3, it has degraded prod at 6) followed by a full `make reindex`. There is
  no incremental path — `is_tech` is not part of `content_hash`.
- Downstream: postings returning to the index become visible to search, facets,
  and `cmd/search-ping`. No schema or API change.
- Design record: `docs/superpowers/specs/2026-09-19-nontech-derive-coverage-design.md`.
