## Why

Issue #2869 proposes three new "aggregator" ingest sources, one of them (`githublists`)
scraping the community new-grad/internship list repos (`SimplifyJobs/New-Grad-Positions`,
`SimplifyJobs/Summer2027-Internships`, `vanshb03/Summer2027-Internships`) as a standing
crawler. Measured against the live catalog, 2,338 of the 2,515 (provider, board) pairs those
repos currently list (93%) are boards we already track — once a company's ATS board is in
our catalog, our regular crawl already surfaces every open req on it, new-grad postings
included, so a permanent crawler over these lists would mostly rediscover what we have. The
genuinely new slice — 177 boards across 9 providers — is real value, but it is a board-catalog
gap, not a missing ingest source, and `cmd/harvest-boards` (the live tool that inserts
discovered boards into the `boards` table, e.g. its Gupy global-feed and OpenCATS scan-index
discovery sources) is where that kind of gap already gets closed.

The repo's older Python board-discovery scripts (`scripts/harvest_boards.py`,
`scripts/discover_boards.py`, `scripts/ats_boards.py`) predate `cmd/harvest-boards` and the
`boards` table; their dedup and `--write` target `sources/<provider>.yml`, a directory
retired by #2406. That dedup is now a silent no-op — every swept candidate looks "new" even
when it is already a tracked board — so extending them for this is picking up a tool whose
core check is broken, when the correct tool already exists and already has the right shape
for this exact kind of discovery.

## What Changes

- `cmd/harvest-boards` already accepts exactly this kind of input — a provider plus a seed
  file of candidate board tokens — for the nine providers this source touches
  (greenhouse/ashby/lever/smartrecruiters/workable/jazzhr/bamboohr/jobvite/breezy), all of
  which it live-validates and inserts into `boards` at `status='pending'` through its
  existing dedup path. Nothing about that tool changes.
- Fix `scripts/ats_boards.py` (shared by `harvest_boards.py` and `discover_boards.py`) so its
  candidate dedup reads the live `boards` catalog instead of the retired `sources/<provider>.yml`
  files (#2406), and so its output is a per-provider seed JSON file in
  `cmd/harvest-boards`'s own seed format instead of an append to a directory that no longer
  exists.
- Update `scripts/harvest_boards.py`'s `AGGREGATORS` list to the three repos this source
  actually needs (`SimplifyJobs/New-Grad-Positions`, `SimplifyJobs/Summer2027-Internships`,
  `vanshb03/Summer2027-Internships`), replacing the stale `Summer2026` URLs.
- Run `harvest_boards.py --write` once to produce the seed files, then `cmd/harvest-boards
  <provider> <seed>.json --apply` once per affected provider, to onboard the 177 boards
  already measured as new against the live catalog.
- No change to `cmd/ingest`, the pipeline, or any ingest `source` — this closes a board-catalog
  gap using the tool built for exactly that, it does not add a new crawled source or change
  `cmd/harvest-boards`'s own behavior.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
<!-- none — cmd/harvest-boards's own behavior (what board-harvest specifies) is unchanged;
     only the upstream Python tooling that produces its seed input is fixed. -->

## Impact

- `scripts/ats_boards.py`: `existing_slugs()`/dedup target and `emit_survivors()`'s write
  path change from `sources/<provider>.yml` to a live `boards`-table read (via `psql`,
  `DATABASE_URL`, stdlib `subprocess` — matching the script's existing stdlib-only
  constraint) and a per-provider seed-JSON file. This is shared by `discover_boards.py` too,
  so its output shape is fixed by the same change, though its own discovery channels
  (ddg/google/github-code-search/common-crawl/serpingapi) are otherwise untouched.
- `scripts/harvest_boards.py`: `AGGREGATORS` list updated to the current-season repos.
- One-off operational run (not a new timer/worker): `harvest_boards.py --write` then
  `cmd/harvest-boards <provider> <seed>.json --apply` per affected provider.
- **Note found while scoping this**: `openspec/specs/board-harvest/spec.md` still describes
  the harvest tool as expanding `sources/<provider>.yml` — the pre-#2406 destination — even
  though `cmd/harvest-boards` has inserted into the `boards` table for a while now. That
  staleness predates and is independent of this change (it affects every existing
  requirement in that spec, not just anything touched here), so fixing it is left to a
  separate documentation pass rather than folded into this one.
