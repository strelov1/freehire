# SQL conventions

## Scope
The `internal/db` package — generated sqlc code, hand-written SQL queries, and the connection pool.

## Always true
- `internal/db/*.go` is **generated code** — never edit by hand. It is committed so the repo builds without sqlc installed.
- To change DB access, edit `internal/db/queries/*.sql` (or `migrations/` for schema), run `make sqlc`, and commit the result.
- Handlers use `*db.Queries`, built once in `handler.Register`. Never construct `*db.Queries` inside a handler.
- `migrations/` is the single source of truth for schema — the same dir feeds both sqlc and Postgres initdb.
- `jobs.UNIQUE (source, external_id)` is the dedup key; `UpsertJob` is `ON CONFLICT` on it.
- Migrations apply via Postgres initdb — `migrations/` is mounted into `/docker-entrypoint-initdb.d`, so each `*.sql` runs **once, on first volume init only**. Changing a migration does NOT re-apply to an existing volume — recreate with `docker compose down -v && make up`.
- A new column referencing `users` needs its **own index** — Postgres indexes only the referenced side, so an unindexed reference makes every account deletion scan that table (this is what timed out account deletion against the 19 GB `jobs` table). `TestEveryUserForeignKeyIsIndexed` enforces it.
- Response shapes: lists are `{"data": ..., "meta": {...}}`, single items are `{"data": ...}`, errors are `{"error": msg}`.
- Handlers signal failure by returning an error — `fiber.NewError(status, msg)` for specific codes, or a bare error (e.g. `pgx.ErrNoRows`). The central `handler.RenderError` maps `*fiber.Error`→its code, `pgx.ErrNoRows`→404, FK violation (SQLSTATE 23503)→404, everything else→500. Don't hand-roll per-handler error JSON.

## How it works

sqlc generates Go types and methods from hand-written SQL in `internal/db/queries/*.sql`. The migration files in `migrations/` define the schema; sqlc reads them to generate `models.go` (types) and `*.sql.go` (queries). The generated `*db.Queries` struct holds all DB methods. Handlers receive a pointer to this struct — created once during route registration in `handler.Register` — and never touch pgx directly.

Migrations are raw SQL files applied automatically by Postgres's entrypoint script on first volume init. There is no versioned migration runner yet, so schema changes require recreating the Docker volume.

The connection pool is owned by `internal/database/pgxpool`. Each worker and the server load config via `config.Load` to get `DATABASE_URL`.

## Limitations
- No versioned migration runner yet; needed before the first schema change ships to a persistent DB.
- Parallel branches have produced several `0009_*` migration files (job-analysis, daily-stats, profile-location→renamed `0010_`); harmless because Postgres initdb runs by filename, but a versioned runner is the real fix.
