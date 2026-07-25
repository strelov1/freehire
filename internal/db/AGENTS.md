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
- Response shapes: lists are `{"data": ..., "meta": {...}}`, single items are `{"data": ...}`, errors are `{"error": msg}`.
- Handlers signal failure by returning an error — `fiber.NewError(status, msg)` for specific codes, or a bare error (e.g. `pgx.ErrNoRows`). The central `handler.RenderError` maps `*fiber.Error`→its code, `pgx.ErrNoRows`→404, FK violation (SQLSTATE 23503)→404, everything else→500. Don't hand-roll per-handler error JSON.

## How it works

sqlc generates Go types and methods from hand-written SQL in `internal/db/queries/*.sql`. The migration files in `migrations/` define the schema; sqlc reads them to generate `models.go` (types) and `*.sql.go` (queries). The generated `*db.Queries` struct holds all DB methods. Handlers receive a pointer to this struct — created once during route registration in `handler.Register` — and never touch pgx directly.

Migrations are raw SQL files. On a fresh Docker volume Postgres's entrypoint applies the whole dir once (initdb); on every existing database `cmd/migrate` (package `internal/migrate`) applies pending files in filename order and records them in `schema_migrations (version text PK, applied_at)`. Deploy rule: run `go run ./cmd/migrate` BEFORE deploying code that reads new schema (the same rule migration comments already state). Bootstrap of a pre-runner database is automatic: an empty `schema_migrations` with the `jobs` table present means the schema is already current, so the runner baselines — records all on-disk files as applied without executing them (`-baseline` forces this explicitly). A file that must run outside a transaction (e.g. `CREATE INDEX CONCURRENTLY`) opts out with `-- migrate: no-transaction` in its leading comment block; write such files idempotently (`IF NOT EXISTS` / `IF EXISTS`).

The connection pool is owned by `internal/database/pgxpool`. Each worker and the server load config via `config.Load` to get `DATABASE_URL`.

## Limitations
- Historical parallel branches produced duplicate number prefixes (`0009_*`×2, `0034_*`×4, …). Harmless: initdb and the runner both order by full filename, and `schema_migrations.version` is the filename, not the number. New files take the next free number; never renumber old files (their versions are already recorded on migrated databases).
