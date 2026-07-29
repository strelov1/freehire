# Catalogue pruning — wave 2 handoff (2026-07-28)

Continuation brief. Prod is host-2, `ssh root@89.167.94.146`.

## What shipped today

Two PRs, both **merged and live** (`main` ancestor check passes on prod):

- **#1162 `fix(prune): the board report needs a verdict, not the absence of one`**
  `cmd/prune --boards` now names a board only when at least one of its postings carries
  an `is_tech` verdict and none resolved technical. Boards with no verdict at all are
  **withheld and counted** in the report header. OpenSpec change
  `openspec/changes/fix-board-retirement-verdict/` (validates; task 2.3 is now done in
  fact but still unchecked in `tasks.md` — worth a small PR to tick it and archive).
- **#1163 `feat(classify): second non-tech mining wave`**
  ~60 terms added to `internal/classify/nontech.go` from a `cmd/mine-titles` run.

### Why #1162 existed

`is_tech` is tri-state; `jobderive` leaves NULL rather than coercing. The old report
collapsed NULL into false, so "nobody classified this board" read identically to
"determined non-technical". Measured: **11 023 of 17 841** listed boards (62%) had no
verdict on a single posting, against 10.6% among boards the same run kept — the bias is
structural, because the `is_tech IS TRUE` half of the evidence test needs a verdict to
fire at all.

Three boards it would have retired, all live IT employers: `bamboohr/irishtitan`
(only posting was the placeholder "No Positions Open"; a real `Associate Digital Project
Manager` appeared hours later), `teamtailor/hypehype` (game studio, one "Open
Application"), `greenhouse/hometapjobs`. All three are withheld now — verified on prod.

### Dictionary wave 2 — what was rejected matters more than what was added

Rejected despite being the largest clusters, because they name roles the board exists to
carry: `team lead` (93 sources), `systems engineer` (80), `tech lead` (78),
`technical lead` (65), `software engineering` (55), `vice president`, `senior director`,
`product management`, `data center`.

Rejected as **absent** roles, not non-technical ones — a technical company's talent pool
matches them exactly: `banco de talentos` (29 sources), `general application` (34),
`ausbildung zum`, `jovem aprendiz`, `sökes till`. Also `são paulo` (a city) and
`and older` (tail of an age requirement).

Anchored where the head word is ambiguous: `mental health technician/counselor/therapist`
not `mental health`; `primary care provider/physician` not `primary care`;
`production supervisor/operator/associate` not `production`. `maintenance engineer`
dropped outright — `Software Maintenance Engineer` already stands as a negative in
`nontech_pruning_test.go`. `wealth management` and `client advisor` dropped as ordinary
fintech engineering-title words.

**Invariant changed:** a non-software `…Engineer` no longer stays unknown where its
discipline is named. `TestDerive_IsTech` now asserts `Senior Mechanical Engineer → false`,
with a new case pinning `Drainage Engineer → nil`.

## What ran on prod

| Step | Result |
|---|---|
| Deploy | `Result=success`, health + facet smoke green |
| `backfill-derive` | `scanned=5348125 updated=1222181 slugs_moved=6324 companies_orphaned=20` |
| `prune --boards` (verification) | `withheld 11024`, listed **6 761** — matched the SQL prediction of 11 023 / 6 818 |
| `prune` dry run | 232 534 targets: `title` 232 533, `unknown_at_empty_company` 1 |
| `prune --apply --limit=25000` | deleted **28 735** rows |
| `prune --apply --limit=-1` | scanned 5 345 802, matched 204 088, deleted **204 748** rows |
| `reindex` (facet, full swap) | `indexed=2669824 skipped=0`, `Result=success` |

Guard refusals during the runs: 5 534 telegram + 98 careerspage — sources that cannot be
re-crawled, held back by design.

**Deleted today: 233 483 rows.** `pruned_jobs` went 1 376 195 → 1 609 678 (exact match).

### Catalogue now

```
total 5 191 817   live 2 673 737
no_verdict 3 175 387 (61.2%)   nontech 1 253 718   tech 762 712
jobs index: 2 670 324 docs      disk: 77 GB free
```

`no_verdict` is roughly flat because ingest keeps adding unclassified rows; `nontech`
dropped ~225k, which is the deletion.

## Prod state left behind

- `freehire-reindexw.timer` — **restarted** (was stopped since the previous campaign).
- `semantic-rehydrate2` — **stopped by me** at `indexed=875517` of ~1.45M, to let the
  facet reindex have Meili to itself. Vectors are safe in `jobs.semantic_embedding`;
  `jobs_semantic` index holds 435 886 docs / 10.4 GB. Resume when convenient:
  `reindex --semantic --from-pg`. Never stack it with a facet reindex.
- Scratch tables from the investigation (`prune_retire_check`, `prune_board_stats`,
  `prune_pruned_board`, `prune_board_title`, `prune_board_class`, `prune_board_open`)
  — all dropped.
- Freed ~7.3 GB of disk: journald vacuum 3.5 GB, `syslog.1` 3.7 GB, apt cache 121 MB.
  Not touched: `/var/log/nginx` (feeds `rollup-views`), `/var/backups/freehire` (holds
  the **only** pg dump, 17 GB, no older generations), Docker (5.6 GB of images report as
  reclaimable but 14 containers are running).

## THE trap that cost an hour — read this before running prune again

`release.sh` builds workers from a hardcoded list; **`prune` and `mine-titles` are not in
it**. A hand-built binary therefore belongs to ONE COLOUR, and the next deploy builds the
*other* colour and flips `hire-current` to it — silently reverting those two binaries to
whatever stale build that colour holds. Nothing errors; the path exists, the run starts,
only the counters come out wrong.

Hit today: built into blue at 03:19, an unrelated deploy flipped to green at 12:20, and
the next `--apply` ran a **Jul-26 binary** whose dictionary lacked the new terms —
1 match in 2M rows instead of ~68k. Caught because `refused` held steady while `matched`
collapsed (`refused` is computed before the source gate, so a live dictionary keeps it
moving). No damage: `prune` deletes only after the full scan, so stopping mid-scan cost
nothing.

**Ritual before every hand-built worker run:**

```bash
# 1. build into the ACTIVE colour (and ideally both)
cd /opt/freehire/src/hire-current && sudo -u freehire /usr/local/bin/go build \
  -buildvcs=false -o prune ./cmd/prune
# 2. prove the dictionary is in the binary
grep -c 'store mgr' $(readlink -f /opt/freehire/src/hire-current)/prune   # want 1
# 3. after starting, prove which inode is running
ls -l /proc/$(pgrep -x prune)/exe
```

Note there were **three** deploys today from other people's work (12:20, 14:52, plus
mine at 03:15) — PRs #1166–#1181. Assume concurrent deploy activity.

## Other operational notes

- Long operations **only** via `systemd-run` — but the converse also bites: a dropped
  ssh kills the `psql` *client*, not the Postgres *backend*. One abandoned SELECT ran
  3h51m holding locks before I found it in `pg_stat_activity` and cancelled it.
- `reindex` refuses to start below `REINDEX_MIN_FREE_GB` (70 GiB floor). It is a
  swap-rebuild: `jobs_rebuild` grows to ~48 GB alongside the old index, then swaps and
  frees it. Budget ~19 GB per million documents; only live non-duplicate jobs are
  indexed (~2.65M), not the whole 5.2M catalogue.
- Measure index growth on `du -sm .../meili/indexes/*`, **not** on `df` — Postgres WAL
  under continuous ingest inflates whole-disk figures roughly twofold.

## Evening session (18:40–20:00 UTC) — what changed

### The report has NOT been re-run. It was started twice and stopped twice, on purpose.

1. **First stop — a stacked reindex.** Arriving to run `--boards` I found a facet swap and
   `reindex --semantic --from-pg` running together. Cause: `/usr/local/bin/semantic-after-wave2b.sh`
   watches a **hardcoded PID** (1183232, the previous session's manual reindex). It waited
   for that, slept 180s and launched semantic at 18:35 — while `freehire-reindexw.timer`,
   restored earlier that day, had fired its own facet reindex at 18:33. A PID watchdog is
   blind to the timer, and restoring the timer is exactly what defeated it.

   Stopped the **facet** swap, not semantic: semantic has no resume and had already reset
   `jobs_semantic` (435 886 docs → 0), so abandoning it is the expensive choice. Caught it
   before `jobs_rebuild` existed, so no orphan index. Semantic went 756 → ~8 000 docs/min.

   **`freehire-reindexw.timer` was stopped, and is now handed to a watchdog:**
   `restore-reindexw-timer.service` (script `/usr/local/bin/restore-reindexw-timer.sh`,
   log `/tmp/restore-reindexw.log`) polls until no `reindex`, `backfill-derive` or
   `reindex-companies` process exists, settles 300s, then runs
   `systemctl enable --now freehire-reindexw.timer`. It serialises on STATE, not on a
   pid — that is the whole lesson of the incident. `enable`, not just `start`: the timer
   was found `disabled` while its siblings (recount, rollup-facets, liveness) are
   enabled, so a start-only restore would not survive a reboot.

   Note `Persistent=true` on a `00/3:15` cadence: enabling it after a missed point
   starts a facet reindex **immediately**. That is why the watchdog waits for quiet
   rather than restoring on a clock.

   The older waiter is still alive and will re-enable `freehire-reindex-companies.timer`
   when semantic ends.

2. **Second stop — another session's backfill.** At 18:55 a different ssh session
   (session-227147) started `backfill-derive` for "classify dict change #1194". It rewrites
   `is_tech` and `skills` — the very columns `--boards` reads — so the report in flight was
   scanning a moving target. Stopped it at 19:12. Colours had also drifted: green @ #1191,
   blue @ #1194, `hire-current` flipped to green at 19:06. **Assume other agents are working
   on this box.**

   The binary ritual held: `prune` was built into BOTH colours at 18:45, so the flip changed
   nothing. Note `--boards` does not use the binary's dictionary at all — it reads `is_tech`
   and `skills` from the DB. The dictionary only decides the title rule under `--apply`.

### The shield was measured, and the diagnosis moved

**PR #1200 is MERGED** (squash `a9d62b6a`, branch deleted) — not yet deployed. On a 0.5% prod sample,
of the 2 633 postings the classifier calls non-technical that still carry skills:

| shield | share |
|---|---|
| boilerplate English words (`agile`, `sap`, `assembly`, `restful`, …) | 194 (7%) |
| nothing but non-engineering skills | 978 (37%) cumulative |
| business tools (`salesforce` 316, `sql` 129, `powerbi` 110, `hubspot` 95) | the rest |

41.7% of ALL non-technical postings carry skills. The dominant cause is **not** English-word
collisions: the dictionary deliberately covers recruiting/HR/finance/legal/ops/CS craft, and
`reportBoards` read any tag as technical evidence. That bug lived in `cmd/prune`, not in the
dictionary. PR #1200 gates the 12 boilerplate words AND has the report ask the new
`skilltag.HasEngineering`.

**Left alone deliberately:** `ruleUnknown` (`cmd/prune/rule.go:95`) reads the same any-skill
signal, but there it is a veto that PROTECTS a job from deletion. Tightening it widens the
only hard-delete path and needs its own measurement.

### Two workers are mute for their whole run

Neither `reportBoards` nor `backfill-derive` logs anything between "starting" and "done". On
5.2M rows that is 20+ minutes of silence in which "working" and "hung" are indistinguishable
except through `pg_stat_activity`. Worth a small PR.

## Next steps, in order

0. **Nothing to do for `freehire-reindexw.timer`** — `restore-reindexw-timer.service`
   owns it now (see above). Verify with `cat /tmp/restore-reindexw.log` and
   `systemctl is-enabled freehire-reindexw.timer`; if that unit died, restore by hand.

1. **DONE and QUEUED — deploy landed 19:58 UTC** (`release-1200.service`, `Result=success`,
   flipped to **blue** @ 7ea10864, which carries a9d62b6a plus #1195/#1205 from other
   sessions; `/api/v1/jobs` and `/api/v1/jobs/facets` both 200). `prune` rebuilt into both
   colours — **but green is still on #1195**, so green's `prune` has no `HasEngineering`.
   Do not "fix" that by pulling green: it is the warm rollback target. Run the binary
   ritual against the ACTIVE colour before any prune run, as always.

   `backfill-1200-waiter.service` (`/usr/local/bin/backfill-after-1194.sh`, log
   `/tmp/backfill-1200.log`) waits for the #1194 backfill to clear, then runs
   `backfill-derive` at CPU/IO weight 20 — below the semantic rehydrate, which has no
   resume and must not be starved. It logs "NEXT: prune --boards is now meaningful" when
   done.

   The two watchdogs are sequenced on purpose: `restore-reindexw-timer` settles 300s and
   **re-checks** before enabling the facet timer, while the backfill waiter settles only
   60s — so the backfill is already running by the re-check and the timer stays off. The
   re-check after the sleep is load-bearing, not cosmetic.

2. **`prune --boards` is QUEUED too** — `boards-1200-waiter.service`
   (`/usr/local/bin/boards-after-backfill.sh`, log `/tmp/boards-1200.log`, report lands in
   `/tmp/prune-boards-1200.txt`). It waits for the #1200 backfill, then performs the
   binary ritual **itself** — builds `prune` into whatever colour `hire-current` points at
   and **aborts** unless the result carries `HasEngineering` — before running the report.

   Why it cannot run earlier, both reasons: `skills` is a stored column so #1200 has not
   reached the catalogue yet, AND the in-flight #1194 backfill is still setting
   `is_tech=true` on titles the new terms recognise (Member of Technical Staff, Founding
   Engineer, AI-Native Engineer). Each such row REMOVES a board from the list — a report
   taken now names live IT employers, which is the #1162 failure exactly.

   That output is the input for the retirement PR (move entries to
   `sources/retired/<provider>.yml`).

   The move is no longer manual. **PR #1210** (`1dc31d30`, deployed) makes the report
   print a `CAUTION` line naming the providers its own list would empty. **PR #1212**
   (`f8b98180`, merged) adds `prune --retire`, which performs the move: same list as
   `--boards` (both go through `boardsToRetire`, so they cannot diverge), line-based
   edits so headers and group comments survive, and a refusal — naming the provider —
   for any entry that would empty one.

   Note the colours flipped to blue **twice in a row**: another session deployed in
   between, so the "inactive" colour was blue again both times. Do not assume the
   colour alternates.

3. **The whole chain is queued. Nothing needs a human until the patch exists.**

   ```
   freehire-backfill-derive   (#1194, ANOTHER session)
     └─ backfill-1200-waiter  → backfill-derive-1200        /tmp/backfill-1200.log
          └─ boards-1200-waiter → prune-boards-1200         /tmp/prune-boards-1200.txt
               └─ retire-1200-waiter → --retire in a clone  /tmp/retire-1200.patch
   restore-reindexw-timer     → enables the facet timer when all of it is quiet
   ```

   `boards-1200-waiter` performs the binary ritual itself and **aborts** unless the
   `prune` it just built carries `HasEngineering`. `retire-1200-waiter` works in a
   **scratch clone** — never hire-blue/hire-green, whose working trees are deployment
   artefacts that `release.sh`'s `git pull` would then refuse.

4. **The one thing left for a person: apply the patch and open the PR.**
   This host cannot push — no `gh`, no key for the `freehire` user, anonymous HTTPS
   remote — so the chain stops at an artefact by necessity, not by choice.

   ```bash
   scp root@89.167.94.146:/tmp/retire-1200.patch .
   git checkout -b chore/retire-boards-wave2 && git apply retire-1200.patch
   ```
   Read `/tmp/retire-1200.log` first: it records how many entries moved and which
   providers were held back. **Spot-check a sample of the moved boards before merging**
   — #1162 exists because a mechanical reading of this report retired live IT employers.
2. **After any retirement PR ships, run `prune --apply` again** — a retired board stops
   being crawled but its jobs are not closed automatically, and only then do the
   company-scoped rules become armed against them.
3. **Decide the borderline five:** `quality engineer`, `process engineer`,
   `project engineer`, `service engineer`, `service technician` — ~30k rows, mostly
   industrial but they collide with QA and IT field-service titles.
4. **Academic IT teaching — open question.** `adjunct faculty` / `assistant professor`
   caught 255 genuinely IT titles (`Assistant Professor, Cybersecurity`,
   `Adjunct Faculty – Computer Science (Programming)`) out of 5 366 academic ones —
   0.11% of the plan. Accepted for now on the grounds that teaching is not practising.
   The clean fix, if wanted, is extending `jobderive.TechEvidence` with discipline names
   (also fixes `Information Technology Support Specialist`), which needs its own
   measurement.
5. **Resume semantic** when the facet work is quiet.
6. **Tick task 2.3 and archive** `openspec/changes/fix-board-retirement-verdict/`.
7. **Consider a changelog entry** — 233k non-IT postings removed is user-facing
   (`write-changelog` skill, posts live in `web/src/posts/`).

## Still-unanswered question worth keeping in view

61% of the catalogue carries no verdict, and enrichment is not the answer: the
`Highway Maintenance Supervisor` posting the user flagged had `enriched_at` set an hour
earlier at version 2 and still came out with empty `category` and NULL `is_tech`. Its
`skills` were `{react, sap}` — the skilltag dictionary firing on a road-maintenance
description. That noise is not harmless: `has_skills` is half the evidence test in
`reportBoards`, so **20 855 listed boards holding 475 625 jobs are kept out of the
retire list by the skills dictionary alone**, with no technical posting anywhere. The
`fix/skilltag-boilerplate-terms` branch is the lever for that.
