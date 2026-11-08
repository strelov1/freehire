## Context

The browse list (`JobsView`) fetches via `GET /api/v1/jobs/search`. With no
`sort` and empty `q`, the handler defaults to `created_at:desc` (catalogue
insertion order). Users expect the freshest postings on top; insertion order is
dominated by per-provider crawl timing instead. The Meilisearch index already
exposes `posted_at` and `created_at` as sortable attributes, and the API already
accepts `?sort=&order=` over an allowlist — so the gap is a UI affordance plus a
default flip, not new backend capability.

A complicating fact: ~13 source adapters set `PostedAt = nil` (whole companies:
tbank, vk, mts, aviasales, …). Any ordering that leans on `posted_at` pushes
those dateless jobs down.

## Goals / Non-Goals

**Goals:**

- Let the user pick the sort field in the UI, so the order is explicit and
  predictable.
- Default the browse to freshest-by-posting-date.
- Keep the choice in the URL, consistent with every other filter.

**Non-Goals:**

- No implicit/derived ordering (rejected `COALESCE(posted_at, created_at)`).
- No new sortable fields, no Meilisearch reindex, no DB migration.
- Salary sort, ascending toggles — not in this change.
- The DB-backed `/api/v1/jobs` list is unchanged (the SPA browses via search).

## Decisions

**Explicit UI control over an implicit fallback.** A `COALESCE(posted_at,
created_at)` sort would need a precomputed sortable field on the index (Meili
can't `COALESCE`) and a reindex, and it hides the ordering rule from the user.
An explicit two-option control is simpler to build, needs no index change, and
makes the dateless-job behaviour the user's own visible choice rather than a
surprise. *Alternative considered:* the COALESCE fallback — rejected as magic.

**Default = Date posted (`posted_at:desc`).** Directly answers the original
complaint. The dateless-jobs-sink side effect is accepted as the default's known
trade-off; the user can switch to "Recently added" to see everything by
insertion. *Alternative:* keep `created_at:desc` default — rejected, it leaves
the reported problem unfixed.

**Sort lives in the existing `FilterStore` / URL pipeline.** Add `sort` to the
filter model and its URL (de)serialization, mirroring `q`: omit `?sort=` while
the default (`posted_at`) is active, include it otherwise. The frontend sends an
explicit `sort` only for the non-default choice and relies on the backend
empty-default for the rest — one source of truth for "what is default".

**`order` stays implicit (`desc`).** Both options are newest-first; no ascending
UI, so the frontend never sends `order` and the backend default (`desc`)
applies.

## Risks / Trade-offs

- [Default `posted_at` hides dateless sources lower in the list] → Accepted and
  visible: it is the default, and "Recently added" surfaces them; documented in
  the spec scenario.
- [Frontend default and backend default could drift] → The frontend omits
  `?sort=` for the default and leans on the backend's empty-query default, so
  the default is defined in exactly one place (the handler).

## Migration Plan

Pure code change. Deploy backend + frontend together. No data migration, no
reindex. Rollback is a revert (no persisted state changes).
