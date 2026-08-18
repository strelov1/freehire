## Context

`jobs.company_slug` is the natural key of the company entity, and it is computed as
`normalize.Slug(jobs.company)` — the employer string as the source wrote it. The slug rule is
faithful; the input is not. Aggregator adapters carry the raw employer field ("RingCentral,
Inc.", "Cobblestone Energy, Dubai - UAE."), ATS adapters carry the board's clean brand name, and
each spelling becomes its own company.

`normalize.Slug`'s own doc comment has carried the seam since it was written: *"It deliberately
does not strip legal suffixes (LLC, Inc, ООО); that is a noted future refinement."* That
refinement was then implemented three separate times, for three callers, with three different
token sets — see "There are already three legal-form lists" below. This change does not add a
fourth; it makes one of them the module's rule and deletes the others.

Three constraints shape everything below.

**`companies` is derived.** `SyncCompaniesFromJobs` builds it and `DeleteOrphanCompanies`
removes any row no job references. Merging companies therefore means rewriting
`jobs.company_slug`, and it also means the `companies` table cannot hold a decision that must
outlive a quiet employer.

**`jobs.company_slug_folded` already exists.** Migration 0109 added it — `company_slug` with
hyphens removed, stored as a column because the planner cannot estimate a predicate over an
expression when the values arrive as a parameter (271s per batch versus 491ms). It is
maintained by the write paths, not the engine, and `internal/db/folded_slug_rule_test.go`
enforces that. The fold this change needs for spelling variants is that same fold, against that
same indexed column.

Measured on prod 2026-08-17 under the rule this design settles on: 5,451 duplicate groups,
11,211 companies, 333,497 open jobs — 24% of the 1,380,940 open catalogue. Split 4,149 groups /
229,004 jobs for legal forms (class 1a) and 1,368 groups / 121,836 jobs for spelling (class 1b).

## Goals / Non-Goals

**Goals:**

- Stop creating class 1a duplicates at the write path, deterministically and with no state.
- Collapse the existing class 1a and 1b fragments in reviewed, reversible waves.
- Keep every retired `/companies/<slug>` URL reachable via 301.
- Keep the aggregator-coverage gate and the upsert reading the same slug, structurally.
- Leave `internal/jobderive` a pure function.

**Non-Goals:**

- **Class 2 — geographic and subsidiary tails** (`cobblestone-energy` vs
  `cobblestone-energy-dubai-uae`; `hitachi-energy` and its six country clones). A further 6,721
  companies / 69,280 jobs, but "Northrop Grumman UK" is a distinct legal entity, so this needs a
  confidence threshold and human review rather than a rule. Deliberately deferred.
- **Widening the token set beyond what the catalogue shows.** The set is the union already
  present in the module, kept because collision measurement supports each token — not extended
  speculatively. A token earns its place by landing on the right employer in the data.
- **A general company-identity resolver** (domain matching, fuzzy names, rebrands). Out of
  scope; `internal/companyname`'s conservative stance still governs that problem.
- **Changing any response body.** Only a new 301 outcome on one endpoint.

## Decisions

### Class 1b cannot be a pure function, so it needs stored state

`jpmorganchase` and `jp-morgan-chase` are both honest `normalize.Slug` output — one source wrote
"JPMorganChase", another wrote "JP Morgan Chase". Collapsing them requires electing a winner,
and a winner is a decision, not a computation.

Three rules were considered. **Prefer the more-hyphenated slug** is attractive because it is
pure and yields readable URLs, and it is wrong: it elects `domino-s`(1) over `dominos`(14,396)
and `al-fa-bank`(20) over `alfa-bank`(1,617). Hyphens mark corrupted spellings about as often as
correct ones. **First-writer-wins** is pure and perfectly stable, but on the existing catalogue
it freezes whichever variant happens to be older, which is arbitrary. **Highest `job_count` at
merge time, then frozen** elects correctly on every case checked in the data — `dollar-tree`,
`dominos`, `alfa-bank`, `jp-morgan-chase` — and is stable afterwards because the election runs
once. That is the rule.

### The canon lives in its own table, not in `companies`

`DeleteOrphanCompanies` drops a `companies` row the moment no job references it. A canonical
slug recorded there would disappear the day an employer's last posting closes, and the next
variant spelling would open a fresh company — the bug, restored, on a timer.

`company_slug_aliases` is therefore the first non-derived table in the company neighbourhood.
One table serves two reads because they are the same relation viewed from either end:

| Reader | Key | Answers |
|---|---|---|
| Ingest (`pipeline.Runner`) | `folded_key` (indexed) | which canonical slug this spelling belongs to |
| `GET /companies/:slug` | `alias_slug` (primary key) | where to 301 |

An alternative — two tables, a canon registry and a redirect map — separates the
responsibilities more cleanly and costs two places that are obliged to stay consistent, to save
one column. Rejected.

`reason` (`legal_form` | `spelling`) exists so a reversal can target one class. Without it,
"undo the legal-form merges" means re-deriving which rows those were and hoping.

### Class 1b resolves in `pipeline.Runner`, not in `jobderive`

`jobderive.Derive` is a pure function: no context, no database. That is its value — the
deterministic-facets spec leans on it precisely because every write path can call it and get the
same answer. Handing it a resolver would hand it a database and end that.

The pipeline is the right home for a second reason: it already computes exactly this batch.
`distinctCompanySlugs` (`pipeline.go:603`) collects the run's distinct company slugs to ask the
coverage lookup about them. The alias resolution attaches to that existing step, so it costs one
extra query per board run, not one per posting.

### One map, two consumers — the invariant becomes structural

`distinctCompanySlugs`'s doc comment states the current invariant plainly: *"the same derivation
jobderive uses for job.Fields().CompanySlug … so the two agree."* Agreement rests on both sides
calling one pure function and on nobody changing one without the other.

That invariant has already failed once in production. The coverage-gate leak spike concluded the
cause was neither timing nor index lag but the **spelling of the slug**. Introducing a
state-dependent slug into a two-caller pure-function agreement would be reintroducing the same
failure with a bigger surface.

So the resolution map is computed once per run and both consumers read it:

```
board's company names
      │  normalize.CompanySlug            (pure — class 1a)
      ▼
distinct derived slugs
      │  SELECT canonical_slug WHERE folded_key = ANY($1)   (one query per run)
      ▼
map[derived]canonical
      ├──► Coverage.NonAggregatorCompanies   (the gate asks about resolved slugs)
      └──► UpsertJob                          (the same resolved slug is stored)
```

A gate that silently stops matching is indistinguishable from a gate with nothing to skip, which
is why this is a spec requirement and not a code comment.

### The facet index is refreshed by the scheduled reindex, not by `search_outbox`

The obvious delivery path — enqueue every re-keyed job so `cmd/search-drain` pushes it — does
not survive arithmetic. A push to the facet index costs 90-180s **regardless of batch size**,
because Meilisearch re-merges the whole inverted index per push
(`internal/searchdrain/AGENTS.md`). At the 500-row default, 333,497 rows is ~667 pushes ≈ 28
hours of continuous pushing, competing for the disk IO that starves `freehire-web`'s `accept()`
queue. That precise shape produced an ~8-minute outage and an unattended multi-hour recurrence
on 2026-08-05.

So the re-key lands in Postgres only, and the already-scheduled `freehire-reindexw` run picks it
up. No manual reindex, and `REINDEX_DEDUP` stays unset — the dedup pass measured ~23h against a
12h unit timeout.

`public_slug` is unaffected: `normalize.JobSlug(in.Title, in.Company, …)` takes the company
NAME, not its slug, so no job URL moves and the stale-Meili-slug 404 spiral is not in play.

### There are already three legal-form lists, and they disagree

This was found while implementing, and it displaced the original plan of "add
`normalize.CompanySlug`". The function exists, and so does `normalize.CompanyKey` — precisely
the folded comparison key this design needed, documented as *"two sources that separate the
words differently still agree."* Both are called from one place,
`cmd/harvest-orphans/candidates.go:47`.

| Definition | Tokens | Repeats | Operates on | Has `co` |
|---|---|---|---|---|
| `normalize.legalSuffixes` | 22 | yes | `Slug` output | yes |
| `collections.legalSuffixes` | 15 | no | name fields, via `letters()` | no, refused explicitly |
| `cmd/harvest-ats.legalFormSuffixes` | 21 | no | slug | yes |

They disagree on substance, not spelling, and each catches what another misses:

| Input | `normalize.CompanySlug` | `collections.RegisterSlug` |
|---|---|---|
| `Booking B.V.` | `booking-b-v` — slug-level, `b v` never matches `bv` | `booking` |
| `Acme GmbH & Co. KG` | `acme` | `acme-gmbh-co-kg` — one pass, no `gmbh`/`kg` |

**Unifying is mandatory, not tidying.** `Collection.Members` looks `RegisterSlug(record.Name)`
up in a map keyed by the catalogue's own company slug. Once the catalogue key strips `gmbh`, a
register row "ACME ROBOTICS GMBH" resolves to `acme-robotics-gmbh` and finds nothing. The
credential is lost silently — the exact failure `internal/collections/AGENTS.md` warns about.

### The wide list wins, on evidence rather than on the comment that forbade it

`register.go:16` refuses `co` because it *"collides with ordinary short words and abbreviations
inside genuine company names."* That reasoning was written for matching register records and
does not survive contact with the catalogue: all 297 companies whose slug ends in `-co` are
`& Co.` forms (Tiffany & Co., Levi Strauss & Co., JPMorgan Chase & Co.), and the strip is
trailing-only, so an interior collision cannot arise.

The decisive test is not whether a token looks dangerous but whether stripping it lands on a
DIFFERENT existing company. Measured on prod 2026-08-17 over the tokens the wide list adds
(`gmbh ab ag kg co oy sa pty srl as`): of the 25 largest collisions, 25 are correct merges —
Accenture GmbH → Accenture, Oracle SA → Oracle, Goldman Sachs & Co. → Goldman Sachs, Ericsson AB
→ Ericsson, JP Morgan Chase & Co. → JP Morgan Chase. Not one lands on an unrelated employer.
(One is right for the wrong reason: `thehivecareers.co` is a domain, not a form, and still
resolves to TheHiveCareers.)

Yield, same measurement:

| Rule | Groups | Companies | Open jobs |
|---|---|---|---|
| Narrow list, single pass | 4,321 | 8,859 | 308,693 |
| Wide list, repeating | **5,451** | **11,211** | **333,497** |

`-gmbh` alone is 2,925 companies the narrow list never touches.

### The unified rule is field-level tokens plus the repeating wide list

Neither existing implementation is adopted whole. Match the trailing form on the name's
whitespace fields reduced to ASCII letters — `register.go`'s `letters()`, which is what makes
`B.V.` a `bv` — and repeat the strip, which is what makes `Acme GmbH & Co. KG` an `acme`.

Two details make that combination work. Fields whose letters are empty (a bare `&`) are stepped
over when looking for the trailing form; that is lossless, because `normalize.Slug` collapses
runs of non-alphanumerics anyway, so `Johnson & Johnson` slugs identically either way. And a
single-field name is never stripped, so `Limited` survives as `limited` — the current
implementation gets this free from its suffixes carrying a leading space, and the field-level
version must keep it deliberately.

Field-level also makes the `" a s"` / `" s a"` entries redundant: `Trafalgar A/S` is one field
whose letters are `as`. They come out, with a test proving the removal inert.

## Risks / Trade-offs

**[Editorial collections are keyed on the old slugs]** → `job-collections` hand lists
(`AICompanySlugs`, `Mag7Slugs`, `BigTechSlugs`, `AINativeSlugs`) and `eastern_roots.txt` (336
entries) match on the raw slug, and `internal/collections/AGENTS.md` warns that a changed rule
silently rewrites membership. Re-key them in the same change and add a guard test that fails if
any entry is not already `CompanySlug`-stable. Credential collections need no change — they
already matched via `RegisterSlug`, and this makes the two rules converge.

**[A wrong merge is user-visible and hard to spot]** → `--apply` is opt-in, waves are bounded by
`--min-jobs`, and every merge is recorded in `company_slug_aliases` with its `reason`, so a class
of merges can be reversed by replaying the rows. The dry run prints the plan for review before
the largest wave.

**[Legal-form strip over-strips a genuine name]** → Only the last field is a candidate, the list
is not extended beyond what collision measurement supports, and a single-field name is left
intact. The behaviour is pinned by unit tests including "Limited Brands", "Limited" and the
punctuated `B.V.` case.

**[Stale facet index between the re-key and the scheduled reindex]** → For up to one reindex
interval, a merged company under-counts its jobs and a retired slug's page 301s to a company
whose list has not caught up. Counts read wrong; nothing 500s or 404s. It self-heals.

**[`reindex-companies` may not be running]** → It has previously skipped silently for 14 days
while reporting success, which would leave `/companies` and the sitemap unaware of every merge.
Confirm the companies index is actually rebuilding before the first wave; this is a
verification step, not an assumption.

**[The catalogue is dirty enough that some pairs will not group]** → `sonsoftinc`(1,612) and
`sonsoft-inc`(1) fold to different keys once the legal form is stripped from one and not the
other, so they will not merge. The change under-merges rather than mis-merges, which is the
recoverable direction; the residue is visible in a later dry run.

**[A new non-derived table in a derived neighbourhood]** → Every other company-adjacent table is
rebuilt from `jobs`. This one is not, by necessity, and that asymmetry has to be documented where
someone will read it — `internal/collections/AGENTS.md`'s sibling for companies, plus the
migration's own comment, following migration 0109's precedent of recording the measurement that
justifies the design.

## Migration Plan

1. **Migration alone.** `company_slug_aliases` is a new empty table, so a plain `CREATE INDEX`
   on `folded_key` — no `CONCURRENTLY`, no lock chain against the nightly dump. Deploys ahead
   of the code, per the standing rule that migrations precede code that reads new schema.
2. **Code.** The table is empty, so alias resolution is a no-op and behaviour is unchanged —
   except that `jobderive` now strips legal forms, so **new** class 1a duplicates stop being
   created immediately, before any backfill.
3. **Waves.** `cmd/merge-companies` dry-run at `--min-jobs 1000`, review, `--apply`. Then 100,
   10, 1. Each wave is chunked and `IS DISTINCT FROM`-guarded, so interrupting one is free and
   re-running writes nothing.
4. **No manual reindex.** Wait for the scheduled `freehire-reindexw`. Do not set
   `REINDEX_DEDUP`, and do not stack a manual `make reindex` against the timer.

**Rollback.** Before any wave: revert the code; the empty table makes it inert. After a wave:
`company_slug_aliases` holds every `(alias_slug, canonical_slug, reason)` that was applied, so
the rewrite can be replayed backwards for a chosen `reason`. The `jobs` rewrite is not
destructive — it moves a key, it does not delete a row, and `pruned_jobs` is not involved.

## Open Questions

None blocking. The class 2 (geographic tail) scope is deferred by decision, not by uncertainty.
