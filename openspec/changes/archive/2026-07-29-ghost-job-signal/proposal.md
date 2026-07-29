## Why

Between 18% and 27% of listings on major boards are postings nobody intends to fill. Greenhouse,
which can see its own customers' hiring pipelines, puts it at 18–22% of jobs posted in any quarter
since 2022, with 70% of its employers having listed at least one. A job seeker cannot tell these
apart from real openings, and every hour spent on one is an hour of unpaid work.

freehire already holds three independent views of the problem and connects none of them:

- `internal/jobreality` classifies a posting `fresh` / `stale` / `likely-evergreen` from ingest
  history and text (PR#501). It is a **posting-shape** signal and knows nothing about outcomes.
- `internal/userjob` derives per-application **silence** against a stage-aware threshold ladder,
  but only on the owner's tracking board — it never aggregates across users.
- `job_reports` accepts a `no_response` complaint, but routes it to a moderator whose only lever
  is closing the job. A complaint that is merely evidence has nowhere to go.

A prior spike (2026-07-21) tried to derive a **ghost-company** badge from structural proxies alone
and was invalidated on prod before shipping. It is the reason this proposal is shaped the way it
is, so its findings are load-bearing rather than history:

- **Age does not discriminate.** Median open-posting age tops out at ~38 days even at p98
  (p50=18, p90=35). Worse, `jobs.created_at` records when *freehire* first crawled a posting, not
  when the employer published it, so age is confounded by how deep our ingest history runs.
- **"Never closes" is an artifact, not a ghost signal.** Companies with `ever_closed_ratio = 0`
  and thousands of open jobs are dominated by staffing/consulting agencies with legitimately
  evergreen pipelines (prosidian, mindlance, avanceconsulting) and by enterprises whose ATS never
  emits a close event (cummins, volkswagen, deloitte). About 34% of companies fired the rule —
  a warning badge on a third of the catalogue, most of it honest.
- **The one unconfounded signal was empty.** The whole catalogue held 194 rows in
  `user_jobs.applied_at`, at most 6 per company. Any response-rate gate was unreachable.

What has changed is not the data volume — it is the shape of the claim. This change does not
assert intent about an employer. It states which of four observable criteria fired, how many
people the evidence came from, and hedges the verdict accordingly. That is also the line the
industry settled on: LinkedIn and Greenhouse answered ghost jobs with *positive* verification
badges precisely because "this posting has been open 240 days" is a verifiable fact while
"this employer is not really hiring" is an assertion about a state of mind.

## What Changes

- **A new `ghost` signal on a job, derived from four criteria in two strength tiers.**
  Structural: `evergreen_posting` (the existing `jobreality` verdict, reused verbatim) and
  `ats_absent` (the posting is on an aggregator but the role is absent from the company's own
  crawled board). Outcome: `silent_applications` (applications tracked here that passed their
  stage's silence threshold) and `user_reports` (people who state they applied and got nothing).
- **Two levels, and structural evidence can never reach the higher one.** `possible` needs
  two criteria of any kind. `likely` additionally needs outcome evidence from **at least two
  distinct people**. A single account cannot move the verdict at all.
- **Silence counts only for a user whose mailbox is connected.** `jobtracking.Silence` falls back
  to `applied_at` when an application has no linked mail, which is right for the owner's own board
  and wrong for a public claim: a user with no connected inbox looks silent even when the employer
  replied to their personal mail. For ghost evidence, absence of a reply must be *observed*, not
  merely unrecorded.
- **A new lightweight report channel, separate from `job_reports`.**
  `POST/DELETE /jobs/:slug/ghost-report` records or retracts one person's "I applied on this date
  and got no answer". No moderation queue, no job close — it is evidence, and a moderated queue
  whose only verdict is "close the job" cannot express that. `job_reports` keeps `fraud`/`spam`
  and its close lever untouched.
- **A new `cmd/ghost-crosscheck` worker** stamps `jobs.ats_absent_at`, gated on the company having
  its own crawled board (absence proves nothing where we never looked) and expiring after 14 days
  (a dead worker stops accusing rather than accusing forever).
- **The verdict is computed at read time, not at index time.** `cmd/reindex` is
  `content_hash`-incremental even at `scope=full`, so a column no adapter writes never reaches
  Meilisearch on its own — the trap `is_tech` already fell into. Ghost evidence changes with no
  ingest at all, so a facet would need its own document-delivery mechanism. v1 serves the verdict
  from Postgres on read; the evidence tables are hundreds of rows.
- **The UI states criteria, never an accusation.** A hedged chip ("Возможно/Вероятно неактивна")
  plus an N/4 scale, and on the job page the full checklist with each criterion's facts and an
  explicit "no data" for the ones that did not fire. The word *ghost* stays internal — package
  name, API field, criterion code — and never reaches the interface.
- **Out of scope:** a Meilisearch `ghost` facet and its exclude filter (noted as a seam; adding a
  filterable attribute opens a hard-500 window until the rebuild swaps, and the verdict is sparse
  at launch); a company-side dispute UI (a moderator can only retract an individual report row by
  hand — an accepted, documented debt, bounded by the mark affecting neither ranking nor
  visibility); and a `reopen_count` signal for "closed and re-posted unchanged", which would need
  new state on the `UpsertJob` hot path.

## Capabilities

### New Capabilities

- `ghost-job-signal`: the four-criteria evidence model, the two-level verdict and its convergence
  and distinct-person gates, the report channel, the served payload with its anonymity gate, and
  the UI's facts-not-accusation presentation.

### Modified Capabilities

- `company-hiring-signal`: gains a per-company application response rate in the existing
  `insights_company_stats` rollup, served only above a sample-size gate.

## Impact

- **Code:** new `internal/ghost` (pure classifier) and `cmd/ghost-crosscheck`; new
  `internal/ghostreport` service + `internal/handler/ghost_reports.go`; `internal/jobview`
  (`ClassifyGhost`, the `Ghost` field, the reality-supersede rule);
  `internal/db/queries/{ghost_reports,jobs,companies}.sql` + `make sqlc`; `cmd/rollup-company`;
  `web/src/lib/ghost.ts`, `GhostBadge.svelte`, `GhostChecklist.svelte`, `JobView.svelte`.
- **Data:** one migration — `ghost_reports` and `jobs.ats_absent_at`. It MUST be applied before
  the image rolls out: the generated `SELECT`s read `jobs.*`, so an unapplied migration 500s
  **every** job read, not just the new feature.
- **Search:** untouched in v1. No new filterable attribute, no rebuild, no 500-window.
- **Risk:** the failure mode of the invalidated spike is staffing agencies, and it survives here —
  `evergreen_posting` fires on them legitimately, and `ats_absent` can fire alongside it when an
  agency advertises a client's role that is absent from its own board, reaching `possible` on
  exactly the companies the previous attempt wrongly flagged. Mitigation is a **calibration gate
  in the rollout, with veto power**: the crosscheck worker is dry-run by default (the `cmd/prune`
  discipline), and the cron is enabled only after a prod report shows who reaches `possible`. If
  staffing dominates, the change stops there and a dict-only exclusion is specified first.
  Until that gate opens the feature is silent by construction: with `ats_absent` unpopulated and
  no reports filed, at most one criterion can fire and two are required — the convergence
  threshold is its own feature flag.
