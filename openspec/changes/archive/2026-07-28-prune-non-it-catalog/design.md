## Context

Prod carries 3.51M open, non-duplicate jobs across 233 087 companies. Only 397 894
(11%) have `is_tech = true`; 1 293 586 (37%) are confidently non-technical and
1 818 399 (52%) are unclassified — the dictionaries are silent on them.

Grouped by `(source, company_slug)` over open jobs:

| bucket | definition | companies | jobs | tech | non-tech | unknown |
|---|---|---:|---:|---:|---:|---:|
| A | no tech title/category, no skills | 127 791 | 430 728 | 0 | 183 653 | 247 075 |
| B | no tech title/category, skills tagged | 51 768 | 485 901 | 0 | 183 226 | 302 675 |
| C | mixed, <10% tech | 10 339 | 1 493 882 | 56 409 | 653 084 | 784 389 |
| D | mixed, 10–50% tech | 20 234 | 921 831 | 223 911 | 251 886 | 446 034 |
| E | mostly tech, ≥50% | 22 955 | 177 537 | 117 574 | 21 737 | 38 226 |

These counts size the problem; the runtime rule classifies a company over its
entire history including closed jobs, so live membership differs.

Bucket C is the trap: 43% of the catalogue at 3.8% tech — hospital systems, retail
chains, municipalities that occasionally post one IT role. A company-level cut
there would destroy 56 409 real technical jobs, so C is out of scope for this
change. Buckets A and B carry zero technical evidence across their whole history
and are the company-level candidates.

The Russian blue-collar cluster is already labelled (`повар`, `уборщик`,
`водитель автомобиля` come back with `nontech = total`). The residual unclassified
mass is dominated by titles the dictionary deliberately refuses to touch:
`registered behavior technician` (~15k across variants), `maintenance`/`service
technician` (~5.1k), `driver`/`car rental driver` (~4.7k), `server` (2.3k),
`team member`/`shift supervisor`/`assistant in training` (~6.2k), and medical
specialities such as `retina specialist ophthalmologist`.

Full analysis: `docs/superpowers/specs/2026-07-25-catalog-pruning-design.md`.

### Measured: the title axis is weaker than the company axis

A spike against prod measured how far title-based mining can actually reach, and
the answer resized the plan.

Whole-title clustering is nearly useless on this data. The 1.81M unclassified jobs
carry **1.06M distinct normalized titles**: 897k jobs (49%) have a title that
occurs exactly once, and 73% sit in titles with fewer than ten jobs. Boards append
location, schedule and requisition detail, so one role splits across many
singletons — `personal care aide - caregiver - on-call - weekdays - honolulu` and
`personal care aide - on-call - waipahu/ ewa beach` are the same job, counted
apart. The top 100 whole titles cover **6.6%** of the mass.

Clustering by word pair, with Unicode tokenization and stop words, raises the top
100's coverage to **15.2%** — 2.3× better, and worth having, but still a minority.
Of those 100 slots, only **44 yield a usable anchored term**; 21 are technical or
IT-relevant phrases an operator must reject (`systems engineer`, `development
engineer`, `principal engineer`, `team lead`, `electrical engineer`), 10 are
boilerplate, and **25 are fragments of a single healthcare employer's verbose
titles** — the same ~3 100 jobs shredded into overlapping pairs.

That last figure is the finding. A quarter of the operator's review budget goes to
one company that a company-scoped rule removes in one mechanical step, with no
dictionary term at all. Buckets A and B hold 550k unclassified jobs on the same
footing.

So the title rule is the *safe* lever, not the *main* one, and the plan is ordered
accordingly: rule (1) iterations prove the machinery and the sampling on
recoverable deletions, and the company-scoped rules carry the volume. The earlier
framing — company rules as a late addition after several title passes — had the
weights backwards.

Cost is capped rather than optimized: **3 minutes 21 seconds** for a full run
against prod, versus 67 seconds for whole titles. Acceptable a few times per
iteration; it is why the report is an operator tool and not a scheduled job.

### Verified on prod

The shipped query was run read-only against prod and the spike's proportions held.
Real anchors led the ranking — `behavior technician` 25 939, `maintenance
technician` 12 111, `service technician` 11 224, `care aide` 3 653, `social
worker` 4 636 — and the connector bridge did its job, surfacing `banco de
talentos` 6 347, a Portuguese talent-pool cluster no two-word group could express.

Two things the run taught that the sample could not:

**Source breadth separates a role from shrapnel.** Every genuine role spans dozens
of sources (`team lead` ~90, `behavior technician` 24) while every fragment of the
one verbose healthcare employer sits on one or two (`afternoon registered`
{apploi}, `lih airport` {apploi,ukg}). The report leads with that count, which
turns an eyeball judgement into a sortable column. It also forced truncating the
source list — ninety names in a cell made the table unreadable.

**One dictionary gap:** `søges til` 3 325 — Danish "wanted for". `til` belongs in
the connector list; the Danish and Nordic function words are not yet covered.

## Goals / Non-Goals

**Goals:**

- Remove non-IT jobs from the catalogue permanently, freeing the disk their
  `description` and `enrichment` occupy and shrinking reindex and rollup cost.
- Make the removal iterative and measurable: one cluster per pass, with the
  remaining unclassified group visibly narrowing.
- Stop the removed jobs from returning on the next crawl.
- Keep every removal auditable after the fact.

**Non-Goals:**

- Bucket-C company pruning. It needs a calibrated "is this an IT company" signal
  (`companies.domains`, YC membership, a validated tech-share threshold); the
  tech-share proxy cannot currently separate a hospital at 11% from a small IT
  shop at 11%.
- Retiring aggregator sources such as `trudvsem` (273k) wholesale — the board is
  one feed, not a company, so that is a product decision, not a rule.
- Serving-layer filtering. This change removes data; it adds no API or SPA filter.
- Feeding the LLM's `enrichment.category` into `jobs.category`. The dict-only
  production convention holds; the LLM's 230 out-of-vocabulary labels stay
  unserved mining material.

## Decisions

**Dictionary expansion, not LLM labelling.** `Sanitize`
(`internal/enrich/enrichment.go:258-265`) deliberately keeps the LLM's category as
unserved discovery material. It is read by humans to find terms worth adding to
`internal/classify`; it never reaches a served column. *Alternative considered:* a
fourth precedence tier in `jobderive` reading the LLM value, which would have
labelled ~130k jobs immediately. Rejected because it makes a served facet
non-deterministic and breaks the stated dict-only convention for a one-off cleanup.

**Physical DELETE, not `closed_at`.** The disk win is the point, and `closed_at`
means "the employer closed this posting". Overloading it with "not our profile"
would corrupt a lifecycle signal three separate mechanisms already write.
*Alternative considered:* a new `excluded_at` column — rejected as a permanent
schema cost for a one-off campaign, since the rows are meant to be gone.

**Rule evaluated live, not from the `is_tech` column.** `cmd/prune` calls the
dictionary directly on each scanned row. *Alternative considered:* run
`cmd/backfill-derive` after every dictionary change and read the column — rejected
because it puts a full keyset pass over 6.5M rows inside every iteration for no
gain. `backfill-derive` runs once at the end to resynchronise survivors.

**Deletion mirrored via `search.DeleteJobs`, not a full reindex.**
`internal/search/client.go:465` removes documents by primary key.
*Alternative considered:* `make reindex` per iteration — rejected, it is hours per
pass and the campaign has many passes.

**Ingest rejects by title only.** Rules keyed on the company bucket are not applied
at crawl time: the bucket does not exist then, and computing it per posting would
couple the pipeline to a whole-catalogue aggregate. The board-retirement report
covers the company-scoped case instead.

**Company-scoped rules require board retirement in the same step.** Boards are
re-crawled hourly on an unchanged dedup key, so a company-scoped deletion without
removing the board entry is undone within the hour. This is a correctness
constraint, not a recommendation, and it is what makes "disable ingest for this
company" part of the same mechanism rather than a separate feature.

**Batches extend to duplicate clusters.** `jobs.duplicate_of` is the one foreign
key referencing `jobs` with `NO ACTION`; every other reference is `CASCADE` or
`SET NULL`. The scan covers canonical rows, which are exactly the referenced ones,
so `WHERE id = ANY(batch) OR duplicate_of = ANY(batch)` is required. Semantically
right: the duplicates of a cook posting are cook postings.

**Anchored terms only.** `internal/classify/nontech.go:17-23` already forbids bare
`engineer`, `technician`, `analyst`, `driver`, `server`, `warehouse`. Every term
this change adds is a full phrase (`behavior technician`, `car rental driver`).
Each one ships with a positive test and a negative test naming a real technical
title that must not match.

**Archive without payload.** `pruned_jobs` stores identity, title, company slug and
the matching rule — roughly 50 MB for 1.5M rows — while the tens of GB of
`description` and `enrichment` are what the deletion is for. *Alternative
considered:* no archive — rejected, an irreversible deletion with no record makes
"did we remove something we should have kept" permanently unanswerable.

## Risks / Trade-offs

- **A too-broad term silently deletes real IT jobs.** → Every added term carries a
  negative test with a real technical title. `--dry-run` is the default, prints a
  random sample of pending titles, and breaks the batch down by rule and source so
  a single-board-dominated batch is visible. The first live run is capped at ~50k.
  A per-board ingest log line surfaces a board rejecting everything within the hour.
- **Rule (1) deletions are recoverable, company-scoped ones are not.** A crawled
  board re-admits its postings once a term leaves the dictionary (with new ids), but
  a retired board plus its deleted rows is gone until the YAML entry returns. →
  Company-scoped rules stay capped at buckets A and B and ship after several
  title-rule iterations have shown the sampling is trustworthy. This is a
  sequencing precaution, not a demotion: the measurement above puts the volume on
  the company axis, so the title iterations exist to earn confidence in the gates
  before the irreversible rules run — not to do the bulk of the work.
- **Moderator-created jobs are not excluded and are never re-crawled.** They are the
  one case a crawl cannot restore. → Accepted by explicit decision; the archive
  records them and re-adding the exclusion is a single predicate clause.
- **User interactions cascade away.** `user_jobs`, `user_job_analysis`,
  `job_reminders`, `subscription_matches` are `ON DELETE CASCADE`, so a user's saved
  or analysed job disappears from their board. → Accepted: 28 users and 827
  interactions on prod.
- **`is_tech` is absent from `content_hash`** (`internal/jobhash/jobhash.go:31-49`),
  so a flip on a *surviving* row never reaches Meilisearch on its own. → Irrelevant
  for deleted rows, which `search.DeleteJobs` handles; one `make reindex` plus one
  `cmd/backfill-derive` at the end of the campaign covers the survivors.
- **Bucket membership shifts as the campaign deletes rows.** → Buckets are computed
  once per run before any deletion, over full company history including closed jobs,
  so a single run is internally consistent and the next run simply sees a newer
  world.

## Migration Plan

One migration adds `pruned_jobs`. Per the project's initdb-only migration model it
must be applied to prod by hand before the worker first runs.

Per-iteration loop:

1. `cmd/mine-titles --limit=100` — read the residual clusters.
2. PR: anchored terms into `classify.nonTechTitleTerms`, with a positive and a
   negative test each.
3. `release.sh freehire`.
4. `cmd/prune --dry-run --sample=200` — inspect the sample and the breakdown.
5. `cmd/prune --apply --limit=N` — batched delete, index removal, archive insert.
6. `cmd/mine-titles` — confirm the group narrowed; repeat.

Company-scoped iterations additionally strike the reported board entries from
`sources/*.yml` in the same PR as step 2.

End of campaign: `cmd/backfill-derive` once, then `make reindex` once.

**Rollback.** Title-rule deletions self-heal: drop the term, deploy, and the next
crawl re-admits the postings under new ids. Company-scoped deletions roll back by
restoring the board entries and waiting for a crawl. The migration is additive and
needs no rollback.

## Open Questions

None blocking. Two deferred: the calibrated IT-company signal that would unlock
bucket C, and whether `trudvsem` stays in the catalogue at all.
