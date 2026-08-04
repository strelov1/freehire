# Pre-release baseline — prod, 2026-08-02T21:11:43Z

Taken before the change was deployed, so the post-release comparison has something to compare
against. Read-only queries; nothing here changed prod.

## `pg_stat_user_tables`

Counters accumulate from the host-2 restore (~2026-07-22), so ~11 days.

| | `jobs` | `companies` |
|---|---|---|
| `n_live_tup` | 5,703,463 | 308,228 |
| `n_dead_tup` | 1,037,925 (15.4%) | 72,116 (19.0%) |
| `n_tup_upd` | **336,282,873** | **290,514,668** |
| `n_tup_hot_upd` | 210,284,984 (62.5%) | 154,286,723 (53.1%) |
| `n_tup_ins` | 793,908 | 31,211 |
| `autovacuum_count` | 465 | 10,165 |

Updates per live row: **59** on `jobs`, **943** on `companies`.

## Size

Database `hire` = **103 GB**.

| | total | heap | indexes | TOAST |
|---|---|---|---|---|
| `jobs` | 92 GB | 19 GB | 9,780 MB | 63 GB |
| `companies` | 9,002 MB | 7,733 MB | 1,265 MB | 2,048 kB |

Live TOAST content on `jobs` is ~20 GB by sampling (see the measurement doc), so ~43 GB of the
63 GB is bloat. `companies` heap is 7,733 MB for 308k live rows — ~25 KB per row.

## Write rate

Two readings the same day, ~5 h apart (the earlier one is in
`docs/superpowers/specs/2026-08-02-ingest-write-amplification-design.md`). The interval is
approximate — the first reading's timestamp was not captured — so treat these as an order of
magnitude, not a measurement:

| | earlier | 21:11Z | Δ over ~5 h | ≈ per day |
|---|---|---|---|---|
| `jobs.n_tup_upd` | 330,948,733 | 336,282,873 | +5,334,140 | ~25 M |
| `companies.n_tup_upd` | 286,000,570 | 290,514,668 | +4,514,098 | ~21 M |

`companies` taking nearly as many updates as `jobs`, against 5.5% as many rows, is the shape
the guard in `UpsertJob`'s `company_upsert` CTE targets.

## Index check (task 8.3)

All 21 indexes on `jobs` enumerated from `pg_indexes` on prod, matching git exactly. **None
references `last_seen_at`** — not as a key column, not in a partial index's `WHERE` predicate.
Both matter: HOT is defeated by a write to any column an index reads, predicate columns
included. The premise behind the cheap write maintaining no index is confirmed against prod
rather than against the repo, which `migrations/0043` records is not authoritative for this
table's index set.

## What to expect afterwards

Read the per-provider `ingest writes:` line FIRST — it says how far the cheap path actually
reached. The counters below are only interpretable in light of it.

- `companies.n_tup_upd` should fall by orders of magnitude. This one is unconditional: the
  guard does not depend on the cheap path matching anything.
- `jobs.n_tup_upd` should fall toward the rate of genuine content change, which is unknown
  until the hit-rate line is read.
- HOT share should rise, but **gradually** — `fillfactor = 90` only applies to pages written
  from here on, and the existing ones are packed at 100% until a repack.
- `n_dead_tup` should fall on both, helped by the lowered autovacuum threshold on `jobs`.

If these do not move, the diagnosis was wrong and the follow-on `pg_repack` must not be run on
this change's strength.

---

# Post-release, 2026-08-02T21:39Z (partial)

Released to prod (blue, `2c3c9690`). `release.sh` applied migration 0073 as one of its own
steps — prod `reloptions` is now `fillfactor=90, autovacuum_vacuum_scale_factor=0.02`.

## The mechanism works

24 providers reported a cheap-write share within the first 20 minutes. **20 of them at 100%**,
`habr_career` and `himalayas` at 99%, `arbeitsagentur` at 89%. **No provider at 0%** — the
silent-zero failure this change most feared did not occur on any of them.

One outlier: `rippling cheap=1039/1809 (57%)`, from a single run. Unresolved, and its
arithmetic does not reconcile: 770 full writes against ~262 rows carrying a post-release
`updated_at`, and 1,809 saves against 1,575 rows that all have distinct `external_id`s.
Candidate explanations (repeated saves within a run, aliased boards, measurement-window edges)
were not distinguishable from the data to hand. The next runs settle whether 57% is a
transitional first-run figure or a real signal.

## The success criterion in this document was WRONG — corrected here

Stated above: "`jobs.n_tup_upd` should fall toward the rate of genuine content change." It should
not, and it did not. **The cheap write is still an `UPDATE`** and counts in `n_tup_upd` exactly
like the full one. What the change removes is not the update but its cost: maintenance of all 21
indexes, and the TOAST rewrite. The right measures are therefore the **HOT share** and the
**non-HOT update rate**, not the total.

The lifetime HOT figure quoted as the baseline (62.5%) is also the wrong comparison — it averages
in older, worse periods. A window measured immediately before the release already read 75.0%.
Window against window is the honest comparison:

| `jobs` | before, 21:11–21:40Z | after, 22:33–22:42Z |
|---|---|---|
| all updates | 1,270,986 /h | 952,518 /h |
| **HOT share** | **75.0%** | **80.5%** |
| **non-HOT updates** | **317,822 /h** | **185,704 /h** (−42%) |

A non-HOT update is the one that maintains all 21 indexes. Those halved.

| `companies` | before | after |
|---|---|---|
| updates | ~903,000 /h | **26 /h** |

Four updates in nine minutes. The company row has effectively stopped being rewritten — as
expected, since its guard does not depend on the cheap path matching anything.

Caveats: the two windows differ in length and in crawl load, so the absolute rates are noisy; the
HOT *share* is volume-independent and is the sounder of the two. `fillfactor = 90` applies only to
newly written pages, so the HOT share should keep climbing as pages turn over — 80.5% is a floor,
not a ceiling.

**Revised condition for the follow-on `pg_repack`:** judge it on the non-HOT rate and the HOT
share, NOT on `n_tup_upd`. Reading the total would have shown a −25% move that means little, and
could as easily have been read as failure.

## Cheap-path share, measured

Across the fleet, including the heavy boards once they cycled onto the new binary:

| provider | share |
|---|---|
| `workday` (87,684 postings) | 99% |
| `smartrecruiters` | 98% |
| `recruitee` | 95% |
| `nofluffjobs` (18,679) | 100% |
| 20 further providers | 100% |
| `arbeitsagentur` | 89% |
| `rippling` | **57%** |

No provider at 0% — the silent-zero failure this change most feared did not occur.

## Open finding: `rippling` does ~3× redundant work per run

Two runs an hour apart produced byte-identical tallies — `cheap=1039/1809 (57%)` at 21:43:51 and
again at 22:44:12, different PIDs. Deterministic, not transitional.

Reconciled against the rows the second run touched:

| | |
|---|---|
| rows whose `last_seen_at` moved | 1,291 |
| of those, cheap (no `updated_at` move) | 1,030 (tally said 1,039) |
| rows whose content actually changed | **262** |
| full writes reported | **770** |
| saves reported | **1,809** |

So 1,809 saves reached 1,291 distinct rows, and 770 full writes landed on 262 distinct rows —
roughly three full writes per changed row, per run. For the repeats to keep taking the full path
rather than falling to the cheap one, each successive save of a row must carry different content
from the last.

The board file is not the cause: 89 boards, all distinct, no repeated `company`. Settling it needs
a read of the `rippling` adapter — whether it yields a posting more than once per run, and why the
yields differ. This is pre-existing behaviour that the new per-provider line surfaced on its first
day; it was invisible before, and it has also been forcing a redundant search-index push on every
crawl.
