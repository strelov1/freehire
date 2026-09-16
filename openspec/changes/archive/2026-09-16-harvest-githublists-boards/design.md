## Context

See proposal.md - Why. Two independent tools already do everything this needs, and neither
needs new capability:

- `cmd/harvest-boards <provider> [seed.json]` (with `--apply` to persist) already
  live-validates a seed list of board tokens against the matching provider's prober and
  inserts survivors into `boards` at `status='pending'` through `boardcatalog.Insert`'s
  existing dedup (`internal/ingest/boardcatalog/AGENTS.md`: "the three writers are ... and
  the harvest tools"). All nine providers this source touches
  (greenhouse/ashby/lever/smartrecruiters/workable/jazzhr/bamboohr/jobvite/breezy) are
  already probeable — several via a dedicated prober, the rest by running their source
  adapter (`proberFor`'s fallback) — so no new prober code is needed regardless of the
  input source.
- `scripts/harvest_boards.py`'s `harvest_aggregators()` already sweeps a list of external
  JSON URLs with a regex-based `extract_slugs` (`scripts/ats_boards.py`) that recognizes all
  nine providers' URL shapes. What it produces today is unusable only because of where it
  looks for "already known" and where it tries to write survivors — both point at
  `sources/<provider>.yml`, a directory #2406 removed. `existing_slugs()` silently returns
  nothing for every provider (the directory doesn't exist, so `f.exists()` is always false),
  so every run currently reports 100% of candidates as "new" regardless of what the live
  catalog actually has.

## Goals / Non-Goals

**Goals:**
- Make `ats_boards.py`'s dedup check the thing that is actually true today (the `boards`
  table), for both scripts that share it (`harvest_boards.py`, `discover_boards.py`).
- Make the GitHub aggregator sweep's survivors land somewhere `cmd/harvest-boards` can
  consume directly, so onboarding them is `go run ./cmd/harvest-boards <provider> <seed>
  --apply`, not a hand-transcription step.
- Onboard the 177 already-measured new boards from the three current-season repos.

**Non-Goals:**
- No new `cmd/harvest-boards` discovery source or prober — the seed-file path already
  covers this input shape, and adding a `discover()` implementation per provider would
  duplicate what `harvest_aggregators()` already does in Python for no behavioral gain.
- No standing `githublists` ingest provider (see proposal.md - Why: 93% of what these lists
  carry duplicates what the regular crawl of an already-tracked board already surfaces).
- No fix to `openspec/specs/board-harvest/spec.md`'s pre-existing `sources/<provider>.yml`
  staleness — real, but predates this change and touches every requirement in that spec, not
  only the part this change exercises.
- No scheduling/timer for a periodic re-run. This ships as a tool a maintainer can rerun by
  hand each internship season, the same posture as `merge-companies` or the `backfill-*`
  one-offs — not every one-off tool in this repo runs on a timer, and this one has no
  measured cadence yet to schedule against.

## Decisions

**Dedup reads `boards` via `psql`, not a Go/DB driver.** Both scripts declare "stdlib only"
in their own docstrings (the `github` channel already shells out to `gh`), and every other
host-side script in this repo that needs the live catalog does so the same way — reusing
`DATABASE_URL` and shelling out to `psql -t -A -c "select provider, lower(board) from
boards"` is the same shape `ats_boards.py`'s `validate()` already uses for the live ATS
probe requests, just against Postgres instead of an HTTP endpoint. Loaded once per run into
a `{provider: set(board)}` map, mirroring the shape `existing_slugs()` already returns today
— every caller of it is unaffected by the swap.

**Python keeps live-validating before writing the seed, rather than handing
`cmd/harvest-boards` everything post-dedup.** `emit_survivors()` already probes each
candidate against `VALIDATORS[provider]` (the same public endpoints
`internal/sources/<provider>.go` reads) before printing it — that step is unaffected by the
`sources/` retirement and still works. Keeping it means the seed file `cmd/harvest-boards`
receives is already the live-and-open subset, not the full ~2,515-candidate sweep; the Go
tool then re-validates through its own prober before insert, which is redundant work but not
new — `cmd/harvest-boards` has no "trust the seed" mode and every other seed source (a
third-party aggregator dump) is fed to it un-pre-validated, so double-probing an already
narrow, already-live set is the smaller cost against writing a `--skip-probe` mode that
nothing else has ever needed.

**One seed file per provider, at a fixed path
(`scripts/.harvest-seeds/<provider>.json`), not one combined file.** `cmd/harvest-boards`
takes one provider per invocation; `emit_survivors()` already groups survivors by provider
before printing, so writing one file per provider per group is the direct fit — no new
grouping logic, and it makes the follow-up command line (`go run ./cmd/harvest-boards
<provider> scripts/.harvest-seeds/<provider>.json --apply`) mechanical to generate from the
run's own report.

**`scripts/.harvest-seeds/` is gitignored, not committed.** These are regenerated inputs to
a one-off run, not the board catalog's own record — the catalog's own record is `boards`
itself, written by `cmd/harvest-boards --apply`. Committing them would create a second,
immediately-stale copy of what the live table already holds, the exact trap `sources/` was
retired for.

## Risks / Trade-offs

- **The nine providers' probe cost is paid twice per candidate** (once by
  `ats_boards.py`'s `validate()`, once by `cmd/harvest-boards`'s own prober) — accepted above
  as the smaller cost; the candidate set feeding it is already the live-validated ~177-ish
  slice, not the full 2,515.
- **`existing_slugs()` becomes a per-run live query against production** rather than a local
  file read. It is one `psql` round trip for the whole `boards` table (already measured at
  178,675 rows, a few MB of text), read-only, and this script is a hand-run maintainer tool,
  not something scheduled — the load is bounded by how often a person runs it.
- **A provider whose only path to a prober is `proberFor`'s adapter fallback** (no dedicated
  prober) costs a full crawl per candidate rather than one HTTP request, per `main.go`'s own
  warning at that fallback. Whichever of the nine providers hits that path will be slow to
  apply; this is `cmd/harvest-boards`'s existing, documented behavior, not something this
  change introduces.

## Migration Plan

1. Land the `ats_boards.py`/`harvest_boards.py` fix.
2. `python3 scripts/harvest_boards.py --write` (needs `DATABASE_URL`; produces
   `scripts/.harvest-seeds/<provider>.json` for whichever providers still have survivors
   after live-validation and dedup against the current catalog — the count may differ
   slightly from the 177 measured earlier in this conversation, since both the aggregator
   repos and the live catalog have kept moving).
3. For each produced seed file: `go run ./cmd/harvest-boards <provider>
   scripts/.harvest-seeds/<provider>.json --apply`.
4. No rollback needed beyond the ordinary board lifecycle — a wrongly-onboarded board is
   `retired`, never deleted, the same as any other board (`internal/ingest/boardcatalog/AGENTS.md`).
