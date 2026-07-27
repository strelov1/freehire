# Catalog pruning — removing non-IT jobs from the catalogue

Date: 2026-07-25
Status: design, approved for planning

## Problem

The catalogue is 3.51M open jobs (PG, `closed_at IS NULL AND duplicate_of IS NULL`)
across 233 087 companies. Most of it is not an IT job board:

| signal | jobs | share |
|---|---:|---:|
| `is_tech = true` (confident technical) | 397 894 | 11% |
| `is_tech = false` (confident non-technical) | 1 293 586 | 37% |
| `is_tech IS NULL` (dictionary silent) | 1 818 399 | 52% |

The mass arrives from generic ATS boards that carry a company's whole hiring, not
its engineering org: workday 545k, trudvsem 273k, oracle 195k, smartrecruiters
166k, ukg 164k, icims 101k. Top titles in the unclassified slice are `подсобный
рабочий`, `уборщик`, `водитель автомобиля`, `повар`, `caregiver`, `CNA`,
`registered behavior technician`, `hospice aid`.

Goal: remove what is not IT, without removing real IT the dictionaries simply
failed to place.

## Company buckets

Measured over open, non-duplicate jobs grouped by `(source, company_slug)`. The
runtime rule classifies a company over its **entire history including closed
jobs**, so live bucket membership will differ from these counts — a company with
a long-closed engineering role leaves bucket A. The table below sizes the problem;
it is not the input to the rule.

| bucket | definition | companies | jobs | tech | non-tech | unknown |
|---|---|---:|---:|---:|---:|---:|
| A | no tech title/category, no skills, ever | 127 791 | 430 728 | 0 | 183 653 | 247 075 |
| B | no tech title/category, but skills tagged | 51 768 | 485 901 | 0 | 183 226 | 302 675 |
| C | mixed, <10% tech | 10 339 | 1 493 882 | 56 409 | 653 084 | 784 389 |
| D | mixed, 10–50% tech | 20 234 | 921 831 | 223 911 | 251 886 | 446 034 |
| E | mostly tech, ≥50% | 22 955 | 177 537 | 117 574 | 21 737 | 38 226 |

Bucket C is the trap: 43% of the catalogue, only 3.8% tech — hospital systems,
retail chains, municipalities that occasionally post one IT role. A company-level
cut there would destroy 56 409 real technical jobs. Buckets A and B are the
company-level kill candidates: zero technical evidence across their whole history.

## Decisions

- **Dictionary expansion only.** The LLM's `enrichment.category` stays unserved
  discovery material (`Sanitize`, `internal/enrich/enrichment.go:258-265`). It is
  read by humans to find terms worth adding to `internal/classify`; it never
  reaches `jobs.category`. The dict-only production convention holds.
- **Physical DELETE**, not `closed_at`. The disk win (description + enrichment of
  ~1.5M rows) is the point, and `closed_at` means "the employer closed it", not
  "not our profile" — overloading it would corrupt a lifecycle signal.
- **Iterative, cluster by cluster.** Mine the residual unknown mass, add anchored
  terms for one cluster, prune, measure what is left, repeat. Not one large rule
  and one large irreversible run.
- **Catalogue boundary: any role at an IT company.** Sales/HR/finance at Stripe
  stay; everything at a hospital goes. The blue-collar role blocklist takes
  priority over this: a cook is removed everywhere, including at an IT company.
- **No exclusions for user data.** `user_jobs`, `user_job_analysis`,
  `job_reminders`, `subscription_matches` are all `ON DELETE CASCADE` and go with
  the job. 28 users / 827 interactions on prod — accepted. Moderator-created rows
  (`created_by IS NOT NULL`) are likewise not excluded, by explicit decision; note
  they are the one case a crawl cannot restore, so a rule mistake that reaches them
  is permanent.

## Deletion rule

Evaluated per row in `cmd/prune`, live against the current dictionary (not from
the stored `is_tech` column, so an iteration needs no `backfill-derive` pass):

```
DELETE if any of:
  (1) classify.IsNonTech(title)                        -- blue-collar blocklist, every company
  (2) category ∈ NonTechCategories AND bucket ∈ {A,B}  -- business role at a non-IT company
  (3) bucket = A AND is_tech IS NULL                   -- unknown at a company with zero tech evidence

Never deleted:
  - telegram / submitted / linksource sources (not re-crawled, so a filter mistake
    is unrecoverable there)
```

Rule (2) is deliberately capped at buckets A and B. Bucket C's 653k confident
non-tech jobs are left alone until the "is this an IT company" signal is
calibrated — the tech-share proxy is too crude to separate a hospital at 11% from
a small IT shop at 11%. That is a later phase, not this one.

Rule (3) ships only after a few rule-(1) iterations have shown bucket A behaves
predictably on sampled titles.

Buckets are computed over a company's **entire history including closed jobs** —
"never has anything IT" needs maximum evidence — once at the start of a `prune`
run, before any deletion.

### Durability coupling

Boards are re-crawled hourly and the dedup key `(source, external_id)` is
unchanged, so **a deleted job returns within the hour unless ingest also rejects
it**. Therefore:

- Rule (1) is self-sufficient: the same dictionary term both marks existing rows
  and blocks new ones at ingest.
- Rules (2) and (3) have no ingest counterpart — they depend on a company bucket
  that does not exist at crawl time. **They may only be applied to companies whose
  board entries are removed from `sources/*.yml` in the same step.** Otherwise the
  deletion is a no-op that costs a full crawl cycle.

`cmd/prune --boards` prints the `sources/*.yml` entries whose
`normalize.Slug(company)` falls in bucket A or B, ready to be struck out by PR.

Aggregator sources (`trudvsem` 273k, `jobtech`, workday shards) have no
per-company board — the board is one feed. Only rule (1) reaches them; retiring
such a source wholesale is a separate product decision, out of scope here.

### Duplicate clusters

`jobs.duplicate_of` is the one FK with `NO ACTION`. The scan covers canonical rows
(`duplicate_of IS NULL`) — exactly the rows duplicates point at — so a batch
delete would hit an FK violation. Each batch extends to its cluster:
`WHERE id = ANY(batch) OR duplicate_of = ANY(batch)`. Semantically correct: the
duplicates of a cook posting are cook postings.

## Components

Nothing changes in the database schema except one new archive table.

**Reused as-is**

- `classify.IsNonTech` (`internal/classify/nontech.go:133`) →
  `jobderive.deriveIsTech` (`internal/jobderive/jobderive.go:190`) → `is_tech`.
  Growing `nonTechTitleTerms` moves the labelling with no code change in derive.
- `cmd/backfill-derive` already re-derives `is_tech` and counts it in
  `facetsMoved`. Run once at the end of the whole campaign, not per iteration.
- `search.Client.DeleteJobs(ctx, ids)` (`internal/search/client.go:465`) removes
  documents by primary key — no full reindex per iteration.

**New**

1. **Ingest filter.** One predicate, two call sites: between `normalizeJob` and
   `r.Store.Save` in the batch path (`internal/pipeline/pipeline.go:336`) and the
   stream path (`:476`). `normalizeJob` goes through `job.New` → `jobderive.Derive`,
   so the aggregate already carries `IsTech` — no extra work. Rejections increment
   a separate `stats.Rejected`, never `Skipped`, so they do not pollute board
   diagnostics. Lives as a package-private helper in `pipeline`: it is catalogue
   policy, not derivation, and two call sites do not warrant a package.

   Only rule (1) applies at ingest. Rules (2)/(3) depend on a company bucket that
   does not exist at crawl time.

2. **`cmd/mine-titles`** — read-only, run-once. Groups the residual unknown
   (`is_tech IS NULL`, open, non-duplicate) by normalized title, prints top N with
   counts and sources. The operator's eyes between iterations, and the measure of
   whether the group is narrowing.

3. **`cmd/prune`** — the only destructive component. Batched DELETE by the rule
   above, `--dry-run` by default, plus `search.DeleteJobs` on the same ids and an
   insert into the archive table.

4. **`pruned_jobs(id, source, external_id, title, company_slug, rule, pruned_at)`**
   — archive without `description` or `enrichment`, so the disk win (tens of GB) is
   kept while ~50 MB answers the only question an irreversible delete otherwise
   makes unanswerable: did we remove something we should not have.

## Iteration loop

```
1. cmd/mine-titles --limit=100        top residual unknown, reviewed by eye
2. PR: anchored terms → classify.nonTechTitleTerms + a test per term
3. release.sh freehire
4. cmd/prune --dry-run --sample=200   random titles of the pending batch, by eye
5. cmd/prune --apply                  batched DELETE + search.DeleteJobs + archive
6. cmd/mine-titles                    measure how much the group narrowed
   → step 1
```

### First iteration target

The Russian cluster is already labelled — `повар`, `подсобный рабочий`, `уборщик`,
`дворник` come back with `nontech = total` (the earlier RU/PT-BR non-tech detector
work). The actual residual unknown is:

| title | ~jobs | why unknown |
|---|---:|---|
| registered behavior technician (RBT), variants | ~15 000 | "technician" is deliberately excluded |
| maintenance / service technician | ~5 100 | same rule |
| driver, car rental driver | ~4 700 | "driver" collides with device driver |
| server | 2 350 | collides with backend |
| team member, assistant in training, shift supervisor | ~6 150 | generic, no role |
| retina specialist ophthalmologist, clinic community liaison | ~6 150 | medical, not in dictionary |
| new bolivar job | 3 400 | a board's junk title |

Every added term must be **anchored** — `behavior technician`, `maintenance
technician`, `car rental driver` — never the bare word. This is the existing rule
in `nontech.go:17-23` and it is what keeps the detector from shadowing a technical
title.

## Reversibility

- **Ingest filter** — fully reversible: drop the term, the next crawl re-admits
  the postings. Cost of a mistake is hours.
- **Rule (1) deletions** — effectively reversible for the same reason: the rows are
  gone but the crawl restores them once the term leaves the dictionary. Ids change.
- **Rules (2)/(3) plus a struck-out board** — irreversible in practice. Restoring
  means putting the YAML entry back and waiting for a crawl.

## Safety gates

1. `--dry-run` is the default; `--apply` must be explicit.
2. `--sample=200` prints random titles from the pending batch, every iteration.
3. `--limit=N` caps a run. The first live run is capped at ~50 000, not released
   on 1.5M.
4. Dry-run output breaks the batch down by rule and by source. If 90% of a batch
   comes from one board, that is a broken board title, not a real cluster.
5. Per-board ingest log line whenever `Rejected > 0`, with the share. A board that
   suddenly rejects 100% means a term was too broad, and it is visible within the
   hour.

## Testing

- `internal/classify` — for every added term, a positive case **and** a negative
  case with a real IT title that must not match. This is the file's existing
  discipline: a bare `technician` fails review because the "Field Service
  Technician / DevOps" negative test catches it.
- `cmd/prune` — the rule predicate is pure and table-driven: bucket × `is_tech` ×
  category × source → delete/keep, plus cases for the manual-source exclusions and
  the duplicate-cluster extension.
- `internal/pipeline` — rejections land in `Rejected` not `Skipped`; batch and
  stream paths behave identically; `cmd/tg-extract` is unaffected.
- No new integration tests. Batched DELETE is ordinary SQL; the `testcontainers`
  suite in `internal/db` is not warranted for it.

## Known gotcha

`jobhash.Of` (`internal/jobhash/jobhash.go:31-49`) includes `category` but **not**
`is_tech`. Growing `nonTechTitleTerms` moves only `is_tech`, so `content_hash` does
not move and the incremental indexer pushes nothing. Irrelevant for deleted rows
(`search.DeleteJobs` handles them), but it means an `is_tech` flip on a *surviving*
row does not reach Meili until a full reindex. Run `make reindex` once at the end
of the campaign, together with the final `cmd/backfill-derive`.

## Out of scope

- Bucket C company-level pruning (needs a calibrated "is this an IT company"
  signal — `companies.domains`, YC membership, tech share with a validated
  threshold).
- Retiring `trudvsem` or other aggregator sources wholesale.
- Serving-layer filtering. This design removes data; it does not add a filter to
  the API or the SPA.
