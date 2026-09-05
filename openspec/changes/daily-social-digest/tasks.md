## 1. Schema

- [x] 1.1 Add migration `0135_social_digest.sql`: `ALTER TABLE job_daily_views ADD COLUMN page_uniques integer NOT NULL DEFAULT 0` (matching the width of `uniques` beside it, which the two are compared against), and `CREATE TABLE social_digest_posts (day date, channel text, job_id bigint, slot int, published_at timestamptz)` keyed `(day, channel, job_id)` with an index on `(job_id, day DESC)` for the quarantine lookup. Verify with `pnpm check:sql`.

## 2. Split the view signal

- [x] 2.1 Change `viewlog.Aggregate` to return `map[day]map[slug]Counts` (`Counts{Total, Page int}`), keeping the existing `(IP, UA, slug, day)` dedup and the existing bot filter on `KindPage` only. Tests first: a page-only job, an API-only job, a job with both, and a bot page open that lands in neither count.
- [x] 2.2 Update `ApplyDailyView` in `internal/platform/db/queries/viewlog.sql` to carry both deltas — `uniques` additive on the total, `page_uniques` additive on the page count, `jobs.view_count` additive on the total. Run `make sqlc`.
- [x] 2.3 Update `cmd/rollup-views` to pass both counts through. Confirm `go build ./...` finds no other caller.

## 3. Selection

- [ ] 3.1 Register `internal/engage/socialdigest` in the block table in `internal/platform/arch/layering/blocks.go`, and add the package's `AGENTS.md`. Confirm the layering test passes with `go test ./internal/platform/arch/...`.
- [ ] 3.2 Add the day-discovery query (freshest day present in `job_daily_views`) and the candidate query: top page-viewed postings for a day, filtered by `closed_at IS NULL AND duplicate_of IS NULL AND NOT is_private AND ats_absent_at IS NULL`, over-fetching so the editorial rules have room to drop rows.
- [ ] 3.3 Implement day selection with the three-day staleness guard and the explicit-day override. Tests: freshest day chosen, stale day fails the run, empty table fails the run, explicit day bypasses the guard.
- [ ] 3.4 Implement the editorial rules over the candidate list — view floor (10), company cap (2 per `company_slug`), quarantine (7 days, read from `social_digest_posts`), top ten. Tests: one per scenario in the spec, plus the thin-day case that publishes nothing and succeeds.

## 4. Rendering and the publisher seam

- [ ] 4.1 Define `Digest` (the day, the ordered postings with title, company, location, and canonical URL) and the `Publisher` interface. Tests cover the URL built for a posting.
- [ ] 4.2 Implement the ledger write and the publish-once check, keyed `(day, channel)`. Tests: a second run for a published `(day, channel)` publishes nothing; a channel not yet published still publishes.

## 5. Discord

- [ ] 5.1 Implement the Discord webhook publisher against `safehttp`, rendering the digest as an embed. Tests against `httptest`: the payload shape, a non-2xx response surfacing as an error.
- [ ] 5.2 Wire configuration: absent webhook URL disables the channel with no error. Test both branches.

## 6. Worker

- [ ] 6.1 Add `cmd/social-digest` on `worker.Main`/`worker.Bootstrap` with `-dry-run` and `-day`. Dry run renders every configured channel's payload to the log, sends nothing, and writes no ledger row.
- [ ] 6.2 Implement multi-channel dispatch: every configured channel is attempted, one channel's failure does not skip another, and the run exits non-zero if any attempted channel failed.
- [ ] 6.3 Add `deploy/systemd/freehire-social-digest.service` and `.timer` (`OnCalendar=*-*-* 10:00:00 America/Sao_Paulo`, `Persistent=true`), and add `social-digest` to the binary list in `deploy/bin/release.sh`. Record in the change that the host's own copy of `release.sh` must be updated by hand.

## 7. LinkedIn

- [ ] 7.1 Implement the LinkedIn organization-post publisher against `safehttp`, reading the organization URN and access token from config. Tests against `httptest`: the payload shape, and an authentication failure surfacing as a distinctly recognizable error rather than a generic one.
- [ ] 7.2 Wire configuration: absent token or URN disables the channel with no error. Test both branches.

## 8. Verification

- [ ] 8.1 On the production host, check when logrotate runs relative to `rollup-views` at 02:30 UTC, and record the answer in the design's Open Questions.
- [ ] 8.2 Run `gofmt -l .`, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`, and `go test -tags=integration ./...` for the touched packages.
- [ ] 8.3 Run `cmd/social-digest -dry-run` against production data and read the rendered list. This is the check that the editorial constants are right; do not enable the timer before it has been read for several days.
