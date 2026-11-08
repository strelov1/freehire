## Why

A project-wide, adversarially-verified simplification audit found the codebase is
well-disciplined (no real overengineering) but carries a handful of small, concrete
drags: dead code built ahead of a need, a few verbatim-duplicated blocks, one
`SELECT *` on a hot path, and one half-built generic abstraction that lies about
being generic. Clearing these now keeps the MVP architecture clean before more
features land on top of them.

## What Changes

This is a **refactor-only** change. No external behavior, wire shape, or API
contract changes; no `BREAKING` changes. Each item below is an independent task.

Dead code (pure removals):

- Remove the unused `search.Client.DeleteJob` method (zero callers) and its now-orphaned `strconv` import.
- Remove the dead `enrich` `Claimed.Attempts` field and its query plumbing (`o.attempts` in `ClaimEnrichmentBatch` RETURNING is threaded through and never read).
- Remove the unused `pipeline.Runner.Concurrency` knob and its `<= 0` fallback (always `0`, always falls through to `defaultConcurrency`).

Backend dedup / efficiency:

- Collapse the triplicated `{data, meta:{total,limit,offset}}` list-response envelope (`ListJobs`/`ListCompanies`/`SearchJobs`) into one `listResponse` helper next to `pageParams`.
- Add a slim `GetJobIDBySlug` query for the view/apply interaction path so it stops `SELECT *`-ing the full `jobs` row (`description` TEXT + `enrichment` JSONB) on every silent view.
- Replace the two index-aligned parallel slices in `smartRecruiters.Fetch` (`jobs []Job` + `found []bool`) with a single `[]*Job` using `nil` as the skip sentinel.

Frontend (Svelte 5 SPA):

- Inline the pure pass-through `get<T>()` wrapper in `api.ts` into its five `request<T>()` call sites.
- Extract the triplicated input class string into a thin `ui/input.svelte` primitive (mirroring `button`/`badge`).
- Render `JobRow`'s tag/skill chips with the shared `Badge variant="secondary"` instead of hand-rolled spans, matching `JobView`.
- Finish the half-built generic And/Or facet toggle so `FacetSection` no longer hard-wires the skills facet (the backend already speaks the generic `<param>_mode=and` protocol).

Maintenance guard (no infra):

- Add a cheap drift guard between the Go enrichment vocabularies and the web `facets.ts` value lists (a source-of-truth comment, plus a small Go fixture test if warranted) — explicitly **not** a codegen pipeline.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

(none — refactor only; no requirement-level behavior changes. The view/apply,
search, and facet endpoints keep their existing wire contracts byte-for-byte.)

## Impact

- Backend: `internal/search/client.go`, `internal/enrich/runner.go`, `cmd/enrich/store.go`, `internal/db/queries/enrichment.sql` + regenerated `enrichment.sql.go`, `internal/pipeline/pipeline.go`, `internal/handler/{handler,jobs,companies,search,user_jobs}.go`, `internal/db/queries/jobs.sql` + regenerated `jobs.sql.go`, `internal/sources/smartrecruiters.go`. Two `make sqlc` regenerations.
- Frontend: `web/src/lib/api.ts`, `web/src/lib/ui/{input.svelte,index.ts}`, `web/src/lib/components/{JobsView,CompaniesView,JobRow}.svelte`, `web/src/lib/components/facets/{SearchSelect,FacetSection}.svelte`, `web/src/lib/filters.svelte.ts`.
- Tests: existing Go suites must stay green; one new/updated Go test for the slug-id path and one for the vocab drift guard. Web has no unit runner — verified via `svelte-check` + lint.
- No migrations, no config, no dependency changes.
