## 1. Liveness evidence (internal/job/qualityaudit)

- [x] 1.1 Failing tests for `readStatus` (404/410 dead, 2xx same path live, 2xx redirected
      moved, 403/429 refused, 5xx/0 no answer) and for `canaryURL` (digits, uuid, slug-suffixed
      ids; too short or absent → no canary; never equal to the original).
- [x] 1.2 Implement `readStatus` and `canaryURL`.
- [x] 1.3 Failing tests for the robots parser (group for `freehire` over `*`, longest match,
      Allow beats Disallow on a tie, Crawl-delay; 4xx allows, 5xx disallows).
- [x] 1.4 Implement the robots parser.
- [x] 1.5 Failing tests for the liveness auditor against an httptest server: calibrated host
      counts dead/live; an any-id host is uncalibrated and its postings are not fetched; a 429
      ends the host; the budget stops the run; robots disallow; `statusBlind` sources.
- [x] 1.6 Implement the auditor (per-host sequential pacing, bounded host concurrency, shared
      budget).

## 2. Remote judge (internal/job/qualityaudit)

- [x] 2.1 Failing tests: the prompt never carries our tags; an ungrounded quote demotes to
      `unclear`; an invalid enum demotes to `unclear`; weighted source choice without replacement.
- [x] 2.2 Implement the judge and the source choice.

## 3. Storage

- [x] 3.1 Migration: `remote_audit_judgements`. Run `pnpm check:sql`.
- [x] 3.2 Queries: eligible sources, liveness sample, remote counts, remote sample, insert,
      56-day tally. Run `make sqlc`.

## 4. Worker

- [x] 4.1 Failing render test pinning the exposition (absent remote series for an unjudged
      source; per-check timestamp).
- [x] 4.2 `cmd/quality-audit`: no-op without `PROM_TEXTFILE_DIR` unless `-dry-run`; the two
      passes; the textfile `freehire-quality-audit.prom`.
- [x] 4.3 Layering table, AGENTS.md worker entry.

## 5. Gates

- [x] 5.1 `gofmt -l .`, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`.
