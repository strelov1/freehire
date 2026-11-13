# SQL conventions

## Scope
The `internal/platform/db` package — generated sqlc code, hand-written SQL queries, and the connection pool.

## Always true
- `internal/platform/db/*.go` is **generated code** — never edit by hand. It is committed so the repo builds without sqlc installed.
- To change DB access, edit `internal/platform/db/queries/*.sql` (or `migrations/` for schema), run `make sqlc`, and commit the result.
- Handlers use `*db.Queries`, built once in `handler.Register`. Never construct `*db.Queries` inside a handler.
- `migrations/` is the single source of truth for schema — the same dir feeds both sqlc and Postgres initdb.
- `jobs.UNIQUE (source, external_id)` is the dedup key; `UpsertJob` is `ON CONFLICT` on it.
- Complete posting reads use `jobs_read` (`db.Job`), which joins observation state from
  `job_activity`. The physical `jobs` model is `db.JobRow`. Return explicit columns from
  base-table writes; `RETURNING *` or `sqlc.embed(jobs)` would expand legacy columns and
  prevent their later removal. A new `jobs` column also needs a replacement `jobs_read`
  projection; `TestJobsReadCoversEveryJobsColumn` checks it. Read observation predicates
  from activity, including inside writers, so compatible workers can overlap a writer
  cutover. Freshness-dependent closers lock jobs then activity with `FOR NO KEY UPDATE`.
- Writers that need both a posting/activity lock and a search queue entry take the
  activity lock before the queue lock. A legacy `AFTER` bridge runs at statement end,
  after data-modifying outbox CTEs; it cannot establish this order for them. Lock both
  rows explicitly before queueing while legacy and activity-only writers overlap.
- Size backfill batches by actual rows, using a stable keyset, and inspect a representative
  plan on production-sized data before shipping. Numeric ID spans are not row counts:
  `jobs.id` is sparse because conflict updates consume sequence values. Small ID spans
  can produce excessive transactions and random heap reads. Preserve committed progress
  and migration immutability if a running one-off copy proves slower than estimated.
- Fresh volumes get the whole `migrations/` dir via Postgres initdb (mounted into `/docker-entrypoint-initdb.d`, applied once on first init, in filename order). Every existing database (prod included) is migrated by `cmd/migrate` (`go run ./cmd/migrate`), which applies only files not yet recorded in `schema_migrations` (version = filename) — one transaction per file, under a session advisory lock. Changing an already-applied migration does NOT re-apply it; add a new file instead.
- **An EXPANSIVE migration is applied before the deploy that reads it, not with it.** Adding
  a nullable column is safe to run ahead of time and unreadable by the previous binary, but
  the next binary reads it on a hot path — an unapplied column is a `42703` on every request
  that touches it, not a degraded feature. Migration `0068` (candidate geography on `users`)
  is the current example; its own header states the ordering.
- **A nullable column and a `NOT NULL DEFAULT '{}'` column answer different questions.**
  `jobs.countries` is the latter because a job always has a location string and derives it
  synchronously, so `'{}'` means "the dictionary was silent". `users.resume_countries` is
  the former because a user has a third state — no CV, or no current structure — so `NULL`
  means "not known" and `'{}'` means "stated, but unresolvable". Collapsing them would lose
  the dictionary's live coverage metric. Copy the shape only when the states match.
- A new column referencing `users` needs its **own index** — Postgres indexes only the referenced side, so an unindexed reference makes every account deletion scan that table (this is what timed out account deletion against the 19 GB `jobs` table). `TestEveryUserForeignKeyIsIndexed` enforces it.
- **A statement that reads OPEN postings carries `AND NOT is_private`.** A private posting is one user's pasted JD (`internal/job/privatejob`); nothing about the row says so — it is open, derived and company-slugged like any other — so four statements leaked or rewrote one before `TestEveryOpenCatalogueReadExcludesPrivateJobs` walked the query files. A statement that may legitimately see one goes in that test's exemption table with the reason, which is checked against the population rather than taken on trust.

Response shapes and error rendering are the handler layer's concern — see
[../handler/AGENTS.md](../../api/handler/AGENTS.md).

- `apply_forms` keys on `job_id` as its PRIMARY KEY, so "at most one current form per job" is structural — a re-capture replaces and no write path can produce two. Its `payload` keeps the ATS's own vocabulary (field identifiers, option values, question text) unnormalized: those tokens exist to be handed back to the platform that issued them, so mapping them into a freehire vocabulary would be loss, not tidying. `apply_form_outbox` copies `semantic_outbox`'s lease/retry semantics and drops its target-version column — a form has no generations, and re-capture is an operator dropping the stored row, which reopens the enqueue gate.

## How it works

sqlc reads `migrations/` for the schema and the hand-written SQL in
`internal/platform/db/queries/*.sql` for the operations, generating `models.go` (types) and
`*.sql.go` (queries). The connection pool is owned by `internal/platform/database` (package
`database`), which wraps the third-party `github.com/jackc/pgx/v5/pgxpool`; the
server and every worker get `DATABASE_URL` via `config.Load`.

Two migration paths, one ordering rule (filename order, always):

- **Fresh Docker volume** — Postgres's entrypoint applies the whole `migrations/` dir once
  via initdb.
- **Every existing database, prod included** — `cmd/migrate` (package `internal/platform/migrate`)
  applies only files absent from `schema_migrations (version text PK, applied_at)`, one
  transaction per file, under a session advisory lock.

A pre-runner database baselines itself: an empty `schema_migrations` plus an existing
`jobs` table means the schema is already current, so the runner records every on-disk file
as applied without executing it (`-baseline` forces this).

A file that must run outside a transaction (e.g. `CREATE INDEX CONCURRENTLY`) opts out with
`-- migrate: no-transaction` **as the whole content of a comment line** in its leading
comment block — a file that merely mentions the marker in a sentence is not opted out.
Those files run one statement at a time, each in its own implicit transaction, so a
statement that Postgres forbids inside a transaction block actually gets what the marker
promises.

Write them idempotently (`IF NOT EXISTS` / `IF EXISTS`): the record insert is not atomic
with them, so a failure part-way leaves the file executed but unrecorded. `IF NOT EXISTS`
used to be a hazard of its own on a CONCURRENTLY re-run — it steps silently over an index a
timed-out build left at `indisvalid=f`, and the version is then recorded against an index
nothing will use (`migrations/0118`, and the prod incident in `0126`/`0127`). The runner now
checks, after a no-transaction file applies, whether any index THAT FILE NAMES is invalid,
and fails without recording rather than stepping over it.

## Limitations
- Historical parallel branches produced duplicate number prefixes (`0009_*`×2, `0034_*`×4, …). Harmless: initdb and the runner both order by full filename, and `schema_migrations.version` is the filename, not the number. New files normally take the next free number; never renumber old files (their versions are already recorded on migrated databases). A corrective file that must run before an immutable failed migration may reuse the preceding prefix: document its filename ordering and prove the retry path with the migration runner. It must not repeat an already recorded data copy.
