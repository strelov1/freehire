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

## The aggregate effect is NOT yet measurable

| | baseline | T+10 min | T+20 min |
|---|---|---|---|
| `jobs` updates/h | ~1.07 M | ~1.11 M | ~1.09 M |
| `companies` updates/h | ~0.90 M | ~0.48 M | ~0.31 M |

`companies` is down ~66% and still falling — expected, since its guard does not depend on the
cheap path matching anything.

`jobs` has not moved, and it is too early for it to have. Verified rather than assumed: none of
the six largest providers (`greenhouse`, `lever`, `workday`, `ashby`, `smartrecruiters`,
`recruitee`) had produced a single `ingest writes:` line at T+20 min, and 140 ingest units were
in flight, many started before the colour flip and therefore running the old binary. One
`greenhouse` run alone persists ~170k postings; the heavy boards fire on 1–3 h timers.

Early positive sign, weak: HOT share of `jobs` updates within the post-release window was ~75%,
against a 62.5% lifetime figure. Short window, contaminated by old-binary runs.

**The decisive measurement is still ~24 h out**, and its condition stands: if `jobs.n_tup_upd`
has not fallen by then, the diagnosis was wrong and the follow-on `pg_repack` must not be run
on this change's strength.
