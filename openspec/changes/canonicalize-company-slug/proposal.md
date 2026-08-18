## Why

`jobs.company_slug` is `normalize.Slug(jobs.company)` — the employer string exactly as the
source wrote it. ATS boards write a clean brand name; aggregators write a legal name with a
corporate form, or a squashed spelling. So one employer becomes several companies, and the
catalogue splits their jobs, facets, collections and SEO weight across the fragments.

Measured on prod 2026-08-17 across the 152,916 companies that have an open job
(1,380,940 open jobs total):

| Class | Example | Groups | Companies | Open jobs |
|---|---|---|---|---|
| **1a** trailing legal form | `ringcentral` / `ringcentral-inc`; `accenture` / `accenture-gmbh` | 4,149 | 8,462 | 229,004 |
| **1b** squashed spelling | `dollar-tree` / `dollartree`; `jp-morgan-chase` / `jpmorganchase` | 1,368 | 2,744 | 121,836 |

Combined: 5,451 groups, 11,211 companies, **333,497 open jobs — 24% of the catalogue**. The
worst fragments are the most visible ones: `dollar-tree`(22,683)+`dollartree`(283),
`jp-morgan-chase`(3,591)+`jpmorganchase`(3,193), JPMorgan Chase in five spellings,
`bosch-group`(2,231)+`boschgroup`(1,086).

The user-visible damage is concrete. `/companies/ringcentral` carries 66 jobs, two curated
collections, seven countries, industries and company info; `/companies/ringcentral-inc`
carries 2 jobs and nothing else — same employer, and the second page is a dead end that also
competes with the first in search.

## What Changes

- **`normalize.CompanySlug` becomes the catalogue's slug rule.** It already exists, alongside
  `normalize.CompanyKey` (its folded comparison form), and today only
  `cmd/harvest-orphans` calls either. `internal/jobderive` switches to it, which is the whole
  of class 1a and stops new 1a duplicates from being created at all.
- **Three legal-form lists collapse into one.** `internal/normalize` (22 tokens, repeating),
  `internal/collections/register.go` (15, single-pass, refuses `co`) and
  `cmd/harvest-ats` (21, single-pass) each define the same rule differently. Unifying is
  forced, not cosmetic: `Collection.Members` looks a register record's `RegisterSlug` up in a
  map keyed by the catalogue's own slug, so a disagreement silently costs a company its
  credential.
- **The wide, repeating list wins, on measured evidence.** Each existing implementation has a
  hole the other covers — the slug-level one cannot strip `Booking B.V.`, the single-pass one
  cannot strip `Acme GmbH & Co. KG`. The unified rule matches trailing fields by their ASCII
  letters and repeats. `register.go`'s refusal of `co` does not survive the catalogue: all 297
  companies ending in `-co` are `& Co.` forms, and of the 25 largest merges the wide tokens
  create, 25 land on the correct employer and none on an unrelated one. `-gmbh` alone is 2,925
  companies the narrow list never reaches.
- **A `company_slug_aliases` table** records the frozen canonical spelling for class 1b and
  doubles as the redirect map. Class 1b cannot be a pure function: both `jpmorganchase` and
  `jp-morgan-chase` are honest `normalize.Slug` output of what the source actually wrote, so
  collapsing them requires picking a winner, and the winner must be remembered.
- **Alias resolution happens once per board run in `pipeline.Runner`**, and its result feeds
  BOTH the aggregator-coverage gate and the upsert. Today those two agree only because both
  call the same pure function; after this change they agree by construction, sharing one map.
- **`cmd/merge-companies`** — dry-run by default, `--min-jobs N` to roll in waves, `--apply`
  to write. Chunked and idempotent, and it maintains `company_slug_folded` alongside
  `company_slug` as every write path must.
- **A retired company slug 301s to its canonical slug** instead of 404ing. This also closes an
  existing hole: `cmd/backfill-company-names` already re-keys company slugs today via
  `RenameSlugCompany` and leaves the old URLs dead.
- **Curated collection hand-lists are re-keyed** through `CompanySlug`, with a guard test —
  editorial collections match on the raw slug, so a changed slug rule silently rewrites their
  membership.

No **BREAKING** API shape change: response bodies are untouched. `GET /api/v1/companies/:slug`
gains a 301 outcome where it previously only had 200/404.

## Capabilities

### New Capabilities
- `company-slug-canonicalization`: the canonical company-slug rule (legal-form strip), the
  frozen-canonical alias registry for spelling variants, how a new posting's slug resolves
  through it, and the merge worker that collapses the existing catalogue in waves.

### Modified Capabilities
- `companies`: the slug is derived by `normalize.CompanySlug` rather than `normalize.Slug`,
  and a slug retired by a merge resolves to a 301 to its canonical company rather than a 404.
- `aggregator-ats-coverage-skip`: the gate's exact-match comparison SHALL be made against the
  same resolved slug the upsert writes, obtained from one shared per-run map, rather than
  re-derived independently.

## Impact

**Go**
- `internal/normalize` — `CompanySlug` regains field-level tokenization; the module's single
  legal-form token set lives here.
- `internal/collections/register.go` — `RegisterSlug` delegates; `legalSuffixes`,
  `significantFields` and `letters` removed.
- `cmd/harvest-ats/candidates.go` — `trimLegalForm` delegates; `legalFormSuffixes` removed.
- `cmd/harvest-orphans/candidates.go` — today's only caller; behaviour widens with the rule.
- `internal/jobderive/jobderive.go:183` — one call site changes; the package stays pure (no
  `ctx`, no database), which is why 1b is resolved in the pipeline and not here.
- `internal/pipeline` — one batched alias lookup per board run, shared by `distinctCompanySlugs`
  (`pipeline.go:603`) and the upsert path.
- `internal/handler/companies.go:311` — 404 falls through to an alias lookup and a 301.
- `internal/collections` — hand lists (`AICompanySlugs`, `Mag7Slugs`, `BigTechSlugs`,
  `AINativeSlugs`) and `eastern_roots.txt` re-keyed.
- `cmd/merge-companies` — new worker.

**Schema**
- New migration: `company_slug_aliases (alias_slug PK, canonical_slug, folded_key, reason,
  created_at)` plus an index on `folded_key`. New empty table, so a plain `CREATE INDEX` —
  no `CONCURRENTLY`, no lock worth naming.
- `jobs.company_slug` and `jobs.company_slug_folded` rewritten for ~333,497 rows, in chunks.
  The existing guard (`internal/db/folded_slug_rule_test.go`) applies to the new write path.

**Web**
- `web/src/routes/companies/[slug]/+page.server.ts` — propagate the redirect instead of
  `error(404)`.

**Prod rollout**
- Migration deploys ahead of the code. The code ships against an empty table, so behaviour is
  unchanged on day one except that new postings stop creating 1a duplicates.
- Waves: `--min-jobs 1000`, then 100, 10, 1, each dry-run and reviewed first.
- **No manual reindex.** The scheduled `freehire-reindexw` run picks the re-key up;
  `REINDEX_DEDUP` stays unset. Until it runs, Meilisearch's `company_slug` facet is stale, so
  a merged company under-counts its jobs for a few hours — counts read wrong, pages do not
  break, and it self-heals.
- `search_outbox` is deliberately NOT used to deliver this. A push to the facet index costs
  90-180s regardless of batch size (`internal/searchdrain/AGENTS.md`), so ~333k rows at the
  500-row default is ~28 hours of pushes — the exact shape that caused the 2026-08-05 outage.
- Verify before the first wave: `reindex-companies` has previously skipped silently for 14
  days while reporting success. If the companies index is in that state, `/companies` and the
  sitemap will not reflect the merges at all.
