## Why

Catalogue **#16** and **#32**, both verdict *overstated*, and both the same shape once the
overstatement is stripped away: **an operational document describing a mechanism the code does not
have.** That is the most expensive kind of wrong comment, because a reader trusts it and stops
looking — six of the twenty shortlist findings turned out to be exactly this.

**#32.** `internal/migrate` says its lock key "only needs to not collide with other advisory-lock
users, and the project has none". There are two: `cmd/liveness` (`0x66686c76`) and
`cmd/ghost-crosscheck` (`0x66686763`). The one place a fourth worker would look asserted there was
nothing to collide with.

`CLAUDE.md` says "`reindex-companies` and `rollup-views` hold their own flock". **Verified on
production: there is no flock — not in Go, not in the systemd units.** What actually serializes a
cron worker against itself is systemd (`Type=oneshot` will not start a second instance while the
first is active), which protects the *timer* path only. A run started by hand has no lock at all —
and that is precisely the basis of the "never stack `reindex-companies` with `make reindex`"
warning, which the flock story obscured.

**#16.** `cmd/backfill-derive` re-derives regions and cities from free-text location, blanking the
structured geography a moderator stated. Its header enumerates what is deliberately not preserved
and omits those two.

## What Changes

Documentation only — no behaviour, no code path.

- `internal/migrate` lists all three advisory-lock keys, with what each is and whether it blocks,
  so the fourth has somewhere to read. Both other sites point at that list.
- `CLAUDE.md` states what actually serializes cron workers, and that a manual run is unprotected.
- `cmd/backfill-derive` records that regions/cities go the same way as the other structured
  facets, that this is **not this command's decision alone** — `internal/moderation`'s edit path
  already re-derives every facet and passes no overrides, so a title typo fix does the same thing
  — and what the measured blast radius is.

## The measurement is the point of #16

Rather than argue about whether stated geography should be durable, I measured it on production:
**7 manually-authored rows, all 7 still carrying regions.** That settles proportionality — this is
a decision to write down, not a data-integrity incident. The note also says where the fix would go
if the intent ever changes: **both** doors at once, never one.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

(none) — documentation only. `tasks.md` is the real artifact; archives with `--skip-specs`.

## Impact

- `internal/migrate/migrate.go`, `cmd/liveness/main.go`, `cmd/ghost-crosscheck/main.go`,
  `cmd/backfill-derive/main.go`, `CLAUDE.md`.
- **Not done, per the findings' own advice:** no `worker.TryAdvisoryLock` (optional at two
  callers, and `worker` pulls config/database/observability that the migration runner must not
  import), and no shared lock-key package (three constants do not earn one — the list earns a
  place to be read, which is what they got).
