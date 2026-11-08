## Context

Public job endpoints serialize the generated `db.Job` struct directly
(`c.JSON(fiber.Map{"data": job})`), so the struct's JSON encoding *is* the API
contract — and it currently includes the sequential `id`. `GetJob`/`RecordView`/
`MarkApplied` all parse a numeric `:id` path param. `UpsertJob` is the pipeline's
single write path but has **no non-test caller yet** (ingest does not exist); the
only thing that mutates jobs today is the enrichment worker via `SetJobEnrichment`.

The dedup key `(source, external_id)` is already `UNIQUE`. Slug normalization
lives in `internal/normalize` (`Slug`) and is already used for `company_slug`; it
lowercases, collapses non-alphanumerics to single hyphens, trims edge hyphens,
and preserves Unicode letters/digits.

## Goals / Non-Goals

**Goals:**

- Expose every job by a stable, non-enumerable `public_slug`; stop exposing the
  numeric `id` in any public response or route.
- Keep `id` as the internal `BIGINT` PK and FK target (`user_jobs.job_id`) — no
  widening, no churn on the hot join path.
- Slug generation is deterministic from `(source, external_id)` so re-ingest is
  idempotent on the slug.

**Non-Goals:**

- Native `external_id` from a source (no parsers/ingest yet).
- Hiding `source`/`external_id` themselves — out of scope; only the numeric `id`
  is a stated leak. (Noted seam if we later want fully opaque public jobs.)
- A versioned migration runner — still initdb-only.

## Decisions

### 1. Slug shape: `<title>-<company>-<shortcode>`, non-empty segments joined

`buildSlug(title, company, source, externalID)` joins the non-empty results of
`normalize.Slug(title)` and `normalize.Slug(company)` with the shortcode using
`-`. Joining only non-empty segments avoids `title--code` when company is empty
(company can normalize to ""). The shortcode is always present, so the slug is
never empty even if both title and company normalize away.

Lives in `internal/normalize` (new `slug.go` sibling, e.g. `JobSlug`) so it sits
next to `Slug` and is unit-testable without a DB. *Alternative considered:*
compute the slug in SQL inside `UpsertJob` — rejected because it can't reuse
`normalize.Slug`'s exact Unicode behavior and would fork normalization logic.

### 2. Shortcode: lowercased base32 of `sha256(source + "\x00" + external_id)`, 8 chars

`sha256("manual" + 0x00 + "42")` → `base32.StdEncoding` → lowercase → first 8
chars. The NUL separator prevents `("ab","c")` and `("a","bc")` colliding. 8
base32 chars = 40 bits; with title+company also having to match for a slug clash,
collision probability is negligible. *Alternatives:* hex (longer, uglier);
4–6 chars (more collisions). The `UNIQUE` constraint is the backstop — see Risks.

### 3. Schema: new migration `0007_job_public_slug.sql`, column `NOT NULL UNIQUE`

Follows the established one-file-per-feature pattern (0002–0006). At initdb time
the `jobs` table is freshly created by 0001 and empty, so `ADD COLUMN
public_slug TEXT NOT NULL UNIQUE` needs no backfill default. The `UNIQUE`
constraint creates the lookup index `GetJobBySlug` needs — no separate index.

### 4. Read path: a `jobResponse` DTO replaces direct `db.Job` serialization

Introduce a handler-local DTO that carries `public_slug` and omits `id`, used by
both `GetJob` (now `GetJobBySlug`) and `ListJobs`. Enrichment passthrough
(`enrichment` as a raw object, `{}` not null) is preserved — the existing
`serialization_test.go` guarantees move onto the DTO. *Alternative:* a sqlc
`// json:"-"` override on `id` — rejected: it would also hide `id` from the
enrichment worker's internal use and couples the DB model to the wire shape.

### 5. Routing: `:id` → `:slug`; interaction handlers resolve slug → id

`GET /api/v1/jobs/:slug` uses `GetJobBySlug`. `POST /api/v1/jobs/:slug/view` and
`/apply` resolve the slug to the internal `id` (one `GetJobBySlug`, mapping
`pgx.ErrNoRows` → 404 via the central handler) and keep writing `user_jobs` by
the narrow `BIGINT` id. `interactionParams` changes from parsing an int to
resolving a slug; `RecordView`/`MarkApplied` otherwise unchanged.

## Risks / Trade-offs

- **Shortcode collision → write error.** Two different `(source, external_id)`
  pairs hashing to the same 8-char code *and* sharing title+company would violate
  the `UNIQUE` slug on insert. → Mitigated by 40 bits of entropy plus the
  title+company prefix; the constraint turns any residual collision into a loud
  write failure, never silent overwrite. Acceptable for MVP volumes.
- **BREAKING route param change.** Existing `/jobs/123` callers (the SPA) break.
  → The SPA is in-repo and updated as part of follow-up; no external API
  consumers yet. Documented in the proposal as BREAKING.
- **Existing job rows on a persistent volume.** A non-initdb DB with rows would
  reject the `NOT NULL` add. → No migration runner / no persistent prod DB yet;
  recreate the volume (`docker compose down -v && make up`), per existing
  convention.
- **`UpsertJob` slug param is exercised only by tests** until ingest lands. →
  Acceptable; the slug builder and the query are independently tested now, and
  the write path is ready for the first parser.
