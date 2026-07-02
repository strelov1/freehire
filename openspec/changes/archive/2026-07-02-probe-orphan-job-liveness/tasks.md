## 1. Schema + queries

- [x] 1.1 New migration: `jobs.liveness_strikes int NOT NULL DEFAULT 0`
- [x] 1.2 `SelectOrphanLivenessCandidates` query: open jobs (`closed_at IS NULL`) whose `source <> ALL($ats_providers)`; returns id/source/url/liveness_strikes
- [x] 1.3 `MarkLivenessExpired` query: `liveness_strikes = liveness_strikes + 1`, set `closed_at = now()` when the new count `>= 2`, `updated_at = now()`; `ResetLivenessStrikes` query: set `liveness_strikes = 0` (no-op when already 0)
- [x] 1.4 `make sqlc`; tagged integration test: strike increments, closes on the 2nd, healthy resets, board source excluded, closed job excluded

## 2. Classifier (`internal/liveness`)

- [x] 2.1 Curated pattern sets (hard-expired EN/DE/FR, error/listing redirect URLs) + min content-length constant as the single source of truth
- [x] 2.2 `Classify(status, finalURL, bodyText) -> (expired bool, reason string)`; table-driven tests over fixtures (404/410, each expired pattern, redirect, short content, healthy 200, 5xx/403)

## 3. Worker (`cmd/liveness`)

- [x] 3.1 `cmd/liveness/main.go`: load `DATABASE_URL`; derive the ATS provider exclusion set from the `sources` registry; select candidates; fetch each URL (per-probe timeout + bounded concurrency); classify; apply expired/reset update per job
- [x] 3.2 Log a per-close line with the reason code (`http_gone`/`expired_body`/`expired_url`/`insufficient_content`); run-once-and-exit; idempotent/safe to re-run

## 4. Docs

- [x] 4.1 AGENT.md: add `cmd/liveness` to the worker family (Layout + Commands) and the liveness convention note

## 5. Rollout

- [ ] 5.1 Apply the ALTER manually on prod (migration-runner seam stays open), deploy, schedule on cron beside ingest/enrich, verify a synthetic dead orphan URL closes after two runs and leaves list/search while resolving on detail
