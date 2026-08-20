## Context

The catalogue's dictionary facets split across two homes. `seniority`, `category`,
`skills`, `work_mode`, `countries` and `regions` are **stored** in `jobs` columns,
derived through `jobderive.Derive` on every write path and reached on existing rows
by `cmd/backfill-derive`. `roles` and `ai_archetype` are **derived at index time** in
`search.FromJob` (`internal/search/document.go:90-91`) and exist only on the
Meilisearch document — no column, no migration, no backfill.

`roletag.Derive(seniority, category, title)` already receives the raw title at index
time, so a title-only signal needs nothing the indexer does not already hold.

Production measurement, 3,148,859 live postings:

| | Postings |
|---|---|
| unambiguous management marker in title | 171,726 |
| …reachable via `category=management` | 15,760 |
| …reachable via `seniority` in (`lead`,`c_level`) | 46,330 |
| …reachable via either | 59,940 |
| …reachable via neither | 111,786 |
| titles containing "manager" | 378,410 |
| …of which are IC roles (product/project/account/…) | 150,779 |
| IC-ladder titles (`staff`, `principal`, … + engineer) | 7,802 |
| titles saying `individual contributor` | 52 |
| `seniority=lead` total / with a management marker | 116,893 / 3,303 |

## Goals / Non-Goals

**Goals:**
- Make "is this a people-management role?" a filterable question.
- Ship without a migration, a column, or a `cmd/backfill-derive` pass.
- Be honest about what the signal can and cannot show.

**Non-Goals:**
- An `individual_contributor` value. See the decision below.
- Description-derived signals (`direct reports`, `manage a team of`).
- Resolving bare "Manager", or "Lead" in either direction.
- Touching `roletag`'s role slugs. A management role keeps whatever role slug its
  title already earns; this is an orthogonal axis, not a new role.

## Decisions

### Index-time derivation, not a stored column

`role_type` is computed in `search.FromJob` and carried top-level on the document,
following `roles` and `ai_archetype` exactly.

*Why:* the signal is a pure function of the title, which the indexer already has.
A column would buy the ability to serve the value on the job wire shape and to
filter it in Postgres — neither of which anything needs — at the price of a
migration, a `jobderive` change, and a ~15-hour backfill against production.

*Consequence:* existing postings get the value when a reindex next rebuilds the
index, and new ones through incremental indexing. There is no separate backfill to
schedule, and no window where the column exists but is unpopulated.

*Alternative considered.* A `jobs.role_type` column via `jobderive`, matching
`seniority`. Rejected on cost: it is the same doctrine and the same dictionary, but
the storage buys nothing this facet needs. The seam stays open — if the value later
needs to appear on the job wire shape, promoting it to a column is a contained
change.

### One value, and the exclusion carries the other side

`vocab.RoleTypeValues` is `["people_manager"]`.

*Why:* the IC side is not detectable. The confident IC population is ~9,100 postings
(0.3%): 7,802 on the `staff|principal|distinguished|fellow engineer` ladder, 1,291
`member of technical staff`, and 52 stating `individual contributor`. A pill
offering 9,100 results beside one offering 171,726 reads as broken, and the only way
to make it look reasonable — treating "no management marker" as IC — is precisely
the inference `internal/classify`'s dict-only rule forbids.

The three-state chip the SPA already gives every facet supplies the other side for
free: excluding `people_manager` returns postings with no marker. That set genuinely
includes most ICs, and it also includes every management posting the dictionary
missed. Calling it "individual contributor" anywhere in the UI or the contract would
convert a known unknown into a false claim, so the labelling requirement is written
into the spec rather than left to taste.

### Bare "manager" is not a marker; craft-qualified is

`engineering manager`, `data manager`, `qa manager` resolve. Plain `manager` does
not.

*Why:* 150,779 of the 378,410 "manager" titles — 40% — are individual-contributor
roles where the managed noun is not a person: Product Manager, Project Manager,
Program Manager, Account Manager, Marketing Manager. Admitting the bare word would
make the facet wrong for two in five of its own matches.

`internal/classify/dictionaries.go` already curates this exact distinction for the
category facet, disambiguating `product manager`→product, `account manager`→sales,
`engineering manager`→management across some forty hand-written entries. This
package reuses that vocabulary rather than inventing a second one.

*Implementation shape:* an unambiguous-marker list checked first, then a
craft-qualified manager list. A blind-phrase mask, in the shape of
`classify.gradeBlindPhrases`, guards the non-management "… manager" forms so a
phrase reaching the matcher by another route still cannot resolve. Masking happens
before matching, and the mask is cut rather than rejected outright, so a real marker
elsewhere in the same title survives — "Director of Product Management" is a
manager because of `director`, whatever sits beside it.

### "Lead" stays unresolved

*Why:* `seniority=lead` holds 116,893 postings and only 3,303 carry any management
marker. In this catalogue "Lead" names the IC ladder — Tech Lead, Lead Engineer —
far more often than a manager. Resolving it either way is a guess, and the
dictionaries do not guess. It stays unresolved and reachable through the seniority
facet, which is where users already look for it.

*This is the whole reason the two controls sit adjacent in the UI.* Users conflate
grade with management; putting the pills next to each other makes the separation
visible.

## Risks / Trade-offs

**A new Meilisearch filterable attribute hard-500s `/api/v1/jobs/facets` until the
live index declares it — this has fired three times (`created_at` sortable, geo
facets, `is_tech`/PR#663).** → The deploy order is: `PUT
/indexes/{jobs,jobs_semantic}/settings/filterable-attributes` with the current list
plus `role_type`, wait for both settings tasks to finish, *then* flip the binary.
Because this change adds no database column, rollback to the previous colour stays
safe — the hazard that made the `is_tech` incident one-way does not apply here.

**Counts read low until a reindex completes.** → The value is absent on documents
built by the previous binary, so the facet under-reports until the scheduled rebuild
lands. That is a wrong count, not a wrong result: every posting the facet does
return is genuinely a match. The alternative — forcing a manual reindex — collides
with `freehire-reindexw.timer` and is the documented way to lose a rebuild.

**The dictionary will miss managers.** → 171,726 is what the title says, not what
the market holds; a "Senior Software Engineer" who inherits three reports is
invisible here. This is the same bounded honesty every dictionary facet in the
repo has, and it is why the exclusion must never be labelled "individual
contributor".

**`supervisor` and `chief` carry non-tech weight.** → "Shift Supervisor",
"Chief of Staff". Both are genuinely people-management roles, so they are correct
matches even where they are not tech roles; `is_tech` and `category` already exist
to narrow that.

## Migration Plan

1. Merge and build, but do **not** flip the binary yet.
2. Patch `filterable-attributes` on `jobs` **and** `jobs_semantic` with the current
   list plus `role_type`. Wait for both Meilisearch tasks to report success.
3. Deploy (`release.sh`), which health-checks the inactive colour before flipping.
4. Verify `/api/v1/jobs/facets?facets=role_type` answers 200 with a (possibly
   zero) distribution, and that `/api/v1/jobs/facets` without it still answers 200.
5. Let the scheduled reindex populate values. Re-check the distribution afterwards
   against the 171,726 measured here.

Rollback is a colour flip back; no schema state is left behind. The extra
filterable attribute on the index is inert to the old binary.

## Open Questions

None. The vocabulary, the derivation site, the manager-word boundary and the deploy
order are settled above.
