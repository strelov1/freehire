# Worker commands

## Scope

What each `cmd/<name>` worker does, when it runs, what it needs, and the trap behind it. Run one with `go run ./cmd/<name>`; every worker needs `DATABASE_URL`, and `ls cmd/` is the full list. The schedules themselves live in the private `freehire-ops` (`provision/host2/AGENTS.md`).

## Workers

Each worker's entry lives in one of these files, grouped by what it works on. A new worker gets its entry in the file that fits and its name in the list here.

- [Ingest and the job lifecycle](workers/ingest.md): `ingest`, `close-chronic-boards`, `capture-apply-form`, `auto-apply`, `liveness`, `adzuna-liveness`, `hydrate-adzuna-description`
- [Enrichment and search](workers/search.md): `enrich`, `search-drain`, `reindex`, `facetd`, `build-suggestions`, `search-settings-drift`, `serve-probe`, `search-ping`
- [Backfills and company data](workers/companies.md): `backfill-derive`, `backfill-company-type-hint`, `backfill-telegram-prefilter`, `backfill-company-info-wikipedia`, `backfill-catalog-history`, `publish-logo-domains`, `report-company-website-twins`, `merge-companies`
- [Operations and quality](workers/ops.md): `migrate`, `llm-probe`, `queue-metrics`, `quality-audit`, `catalog-coverage`, `report-cv-letters`, `report-skill-gaps`, `report-classify-drift`
- [Users, billing and mail](workers/engage.md): `billing-sync`, `discord-sync`, `pro-welcome-mail`, `cv-lifecycle-mail`, `mentorship-remind`, `message-notify`, `mentor-busy-sync`, `email-events`, `social-digest`, `linkedin-auth`, `linkedin-token-refresh`, `sweep-tracer-clicks`

## Locking

**Nothing holds a flock** — no file locks in Go or in the systemd units; the word survives only in a few stale comments. What keeps a cron worker from stacking on itself is systemd: a `Type=oneshot` unit will not start a second instance while the first is active. That protects the TIMER path only, so a run started by hand has no lock at all. **Never stack `reindex-companies` with `make reindex`** — Meilisearch runs one serial task queue, so the second rebuild queues behind the first and looks like a hang, and a swap transiently holds ~2x the index's disk. Two workers additionally take a Postgres advisory lock (`cmd/liveness`, `cmd/ghost-crosscheck`), and `reindex`, `reindex-companies` and `build-suggestions` share one more (`worker.HeavyIndexLockKey`), however they were started; their keys are listed in `internal/platform/migrate`. A run that finds the index lock held logs `another catalogue-wide index job is running — skipping this run` and exits 0 having done nothing, so re-run a skipped manual rebuild once the other job is done.
