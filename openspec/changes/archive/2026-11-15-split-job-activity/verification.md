# Rollout verification

## Compatibility and deployment

The reader release needed an explicit jobs-before-activity lock order before
overlapping activity-only writers. A PostgreSQL 18 overlap harness reproduced
deadlocks in 15 of 48 initial-reader overlaps; the compatible follow-up completed
all 96 statement outcomes without deadlocks. Executable revisions on core and
operator were inspected before writer and contract cutovers. The old derive
worker was drained with its resume cursor preserved; its full snapshot comparison
finished before the new B-compatible pass started.

Writer restoration [#4580](https://github.com/strelov1/freehire/pull/4580) shipped
as `587c7325407fffc94b22ab5a33b74c19b0f19440` on November 14, 2026, at
10:57:48 UTC. Its isolated +12/+25 comparisons passed against the actual preceding
build: 15/15 smokes, zero API 5xx and no newly failed units.

Contract [#4601](https://github.com/strelov1/freehire/pull/4601) merged as
`3b260d86c802641ebf80475bdff7da5ad7639b66`. Migration
`0266_job_activity_contract.sql` applied at 19:40:54 UTC and the normal release
`7bd5212f2f301f3b95a63cf990740f0d6135f460` flipped at 19:41:03 UTC. The unrelated
Gmail regression had been reverted before this batch. The deploy agent published
the [release report](https://github.com/strelov1/freehire/releases/tag/deploy-20261114-2007).

At +12/+25, all 15 smokes passed and API 5xx remained zero. API p95 was
0.095 seconds before, then 0.087/0.095 seconds. The comparisons were SUSPECT only
because the old enrich process was cancelled at the flip; it resumed at 20:05 UTC
with eight jobs enriched and no failures. The agent found no application
undefined-column errors or deadlocks after the DDL.

A fresh check on November 15 confirmed zero legacy columns, no legacy bridge,
a validated FK, enabled insert/state triggers and the team projection in
`jobs_read`. All 100 most recent posting inserts had an activity row. Installed
operations harvest queries still matched the reviewed hashes; the resumed
B-compatible derive continued through contraction without restarting its snapshot.

## Statistics role follow-up

The source-statistics stage exposed a missing SELECT grant for the existing
`freehire_stats` role on `job_activity` (SQLSTATE 42501).
[Ops #283](https://github.com/strelov1/freehire-ops/pull/283) records the
repeatable grant, applied to the primary on November 15 at 02:08 UTC. The role
can now resolve the activity read on primary and standby; INSERT/UPDATE/DELETE
privileges remain absent. No password, timer or worker restart changed, and no
heavy catalogue rollup was launched to test this ACL. Publication at the next
scheduled run remains a separate observation.

## Physical observations and WAL

In a bounded sample of 1,500 public postings, activity timestamps naturally
advanced for 18 postings. All 18 retained their jobs xmin/xmax/ctid, content hash
and activity state version. An earlier 150-row sample had no repeated
observations and therefore supplied no physical-tuple proof.

Separate pre/post writer traffic windows measured WAL bytes per call:

- Open touch: 5,568.4 before; 323.1 after, over 132,171 post-cutover calls.
- Unchanged refresh: 6,265.2 before; 420.5 after, over 77,656 post-cutover calls.

These are observational windows with different FPI/checkpoint, provider, load
and cache conditions. They are not a controlled causal estimate, a daily savings
forecast or evidence of faster reads. Table HOT ratios aggregate mixed callers.
Dropping the columns did not rewrite or reclaim the existing jobs heap.

## Read cost and limits

Production keyset pages of 50 postings performed 50 activity lookups, without a
prefix walk. Scoped board and provider samples returned matching populations;
some read paths used approximately 5.8 times the logical buffers. That overhead
was accepted for this persistence change, which did not tune those readers.
Base-first/view-second warm-cache timings are not speedup evidence. Broad and
batched cold Workday queries timed out for both old and new reads; their
throughput remains unmeasured.

The long derive is separate catalogue maintenance. Its safe resumed execution
and preserved full comparison are verified; completion of that whole pass is
not a condition for the observation-persistence rollout.
