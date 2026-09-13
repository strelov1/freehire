## Why

A candidate has no way to tell, before spending a tailoring session on a posting,
whether its ATS is even one `cmd/auto-apply` can attempt at all. A production
review of the auto-apply queue found a single lifetime successful submission, and
most parked entries fail on a provider gap (Recruitee has no fill/submit path at
all) or a form the browser driver cannot recognize — failures that are knowable
in advance from the job's own `source`, before any tailoring or review happens.
Surfacing that as a filter lets a candidate self-select toward postings where
auto-apply is at least provider-eligible, instead of discovering the gap after
the fact.

## What Changes

- Add a new, catalogue-wide, true-or-absent job facet, `auto_apply_available`:
  `true` when the posting's `source` is one of the four ATS platforms
  `internal/api/atsapply` can currently drive (`greenhouse`, `lever` via chromedp;
  `ashby`, `workable` via the browser-use cloud-agent fallback — included even
  though that fallback ships operationally OFF by default today, per explicit
  product decision to frame the filter as best-effort, not a guarantee), absent
  for every other source (Recruitee included — it has no fill/submit path).
- Compute it purely from the already-stored `jobs.source` column, inline in the
  shared Meilisearch document builder (`search.FromJob`) — no new Postgres
  column, no migration, no backfill worker. It self-heals on every future
  incremental index push; a one-time full `make reindex` is still required to
  backfill the attribute onto already-indexed open postings, the same
  settings-before-binary-before-reindex sequencing every filterable attribute in
  this codebase already follows.
- Expose it as a public, documented `GET /api/v1/jobs?auto_apply_available=true`
  filter (and in `/jobs/facets`), following the exact `requires_clearance` /
  `ai_interview` true-or-absent shape.
- Add a bespoke UI checkbox, "Auto-apply available" (with a "Best-effort —
  successful submission isn't guaranteed" caption), following the existing
  "Offers visa sponsorship" / "Hide employers reported to interview with AI"
  checkbox pattern — not the generic facet-rail system, which deliberately never
  exposes raw `source` to candidates.

Explicitly out of scope: any change to `internal/api/atsapply`'s own provider
maps, to the auto-apply worker/queue logic, or to whether the Ashby/Workable
browser-use fallback is enforced in production; and the public filter-docs page
(this checkbox follows `visa`/`hideAIInterview`, which are already absent from
that page).

## Capabilities

### New Capabilities
- `auto-apply-availability-facet`: what "auto-apply available" means, the fixed
  provider set, where and how it is computed (index-build time, not a stored
  column), and how it is served/filtered/rolled out.

### Modified Capabilities
- `filter-modal`: adds the "Auto-apply available" checkbox alongside the
  existing bespoke boolean checkboxes.

## Impact

- `internal/search/search/document.go` — new `JobDocument.AutoApplyAvailable`
  field, computed in `FromJob`.
- `internal/search/search/client.go` — new Meilisearch filterable attribute.
- `internal/search/search/query_filter.go` — new `StringFacets` entry, new
  true-or-absent param, new exported `AutoApplyAvailableParam` constant.
- `web/src/lib/facetModel.ts`, the filter modal's Svelte component — new
  checkbox field, serialization, and control.
- Rollout: Meilisearch settings patch, then the binary, then one full
  `make reindex` (operational step, no code backfill).
