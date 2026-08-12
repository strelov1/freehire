# SQL conventions

## Scope
The `internal/db` package — generated sqlc code, hand-written SQL queries, and the connection pool.

## Always true
- `internal/db/*.go` is **generated code** — never edit by hand. It is committed so the repo builds without sqlc installed.
- To change DB access, edit `internal/db/queries/*.sql` (or `migrations/` for schema), run `make sqlc`, and commit the result.
- Handlers use `*db.Queries`, built once in `handler.Register`. Never construct `*db.Queries` inside a handler.
- `migrations/` is the single source of truth for schema — the same dir feeds both sqlc and Postgres initdb.
- `jobs.UNIQUE (source, external_id)` is the dedup key; `UpsertJob` is `ON CONFLICT` on it.
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

Response shapes and error rendering are the handler layer's concern — see
[../handler/AGENTS.md](../handler/AGENTS.md).

- `apply_forms` keys on `job_id` as its PRIMARY KEY, so "at most one current form per job" is structural — a re-capture replaces and no write path can produce two. Its `payload` keeps the ATS's own vocabulary (field identifiers, option values, question text) unnormalized: those tokens exist to be handed back to the platform that issued them, so mapping them into a freehire vocabulary would be loss, not tidying. `apply_form_outbox` copies `semantic_outbox`'s lease/retry semantics and drops its target-version column — a form has no generations, and re-capture is an operator dropping the stored row, which reopens the enqueue gate.

## How it works

sqlc reads `migrations/` for the schema and the hand-written SQL in
`internal/db/queries/*.sql` for the operations, generating `models.go` (types) and
`*.sql.go` (queries). The connection pool is owned by `internal/database` (package
`database`), which wraps the third-party `github.com/jackc/pgx/v5/pgxpool`; the
server and every worker get `DATABASE_URL` via `config.Load`.

Two migration paths, one ordering rule (filename order, always):

- **Fresh Docker volume** — Postgres's entrypoint applies the whole `migrations/` dir once
  via initdb.
- **Every existing database, prod included** — `cmd/migrate` (package `internal/migrate`)
  applies only files absent from `schema_migrations (version text PK, applied_at)`, one
  transaction per file, under a session advisory lock.

A pre-runner database baselines itself: an empty `schema_migrations` plus an existing
`jobs` table means the schema is already current, so the runner records every on-disk file
as applied without executing it (`-baseline` forces this).

A file that must run outside a transaction (e.g. `CREATE INDEX CONCURRENTLY`) opts out with
`-- migrate: no-transaction` in its leading comment block — write those idempotently
(`IF NOT EXISTS` / `IF EXISTS`).

## Limitations
- Historical parallel branches produced duplicate number prefixes (`0009_*`×2, `0034_*`×4, …). Harmless: initdb and the runner both order by full filename, and `schema_migrations.version` is the filename, not the number. New files take the next free number; never renumber old files (their versions are already recorded on migrated databases).
