## Context

See `proposal.md` - Why. Today's board sharing lives in `internal/search/savedsearch` (`share.go`: `Share`/`Unshare`/`GetPublicBoard`, plus the `boardSlug()` minter — transliteration + 4-char random suffix, retried on collision), backed by `saved_searches.public_slug`/`author_label`, `GET /api/v1/boards/:slug`, and the SvelteKit route `web/src/routes/b/[slug]`. `saved_searches` itself (private CRUD, name/query, the `/my/notifications/searches` account page) is unaffected and stays.

The existing per-job "save" (`internal/application/jobtracking`, `user_jobs.saved_at`) is a single boolean-like flag per `(user_id, job_id)` with no grouping — confirmed to have no existing "named set of specific jobs" concept anywhere in the codebase. `internal/job/collections` is an unrelated, code-owned company-level curated registry (Big Tech/YC/visa-sponsor tags feeding the `jobs.collections` search facet and the `/collections` hub) — its name collides with the natural word for this feature, which is why the new capability is named **job lists**, not "collections", both in the OpenSpec capability path (`saved-job-lists`, distinct from the existing `job-collections` capability) and in code.

## Goals / Non-Goals

**Goals:**
- Replace board sharing with a strictly simpler primitive: a named set of specific jobs, optionally shared read-only by slug.
- Reuse the slug-minting approach (transliteration + random suffix + collision retry) that boards already proved out, rather than inventing a new scheme.
- Keep the new feature fully independent of the existing "save" flag and of `saved_searches`.

**Non-Goals:**
- No per-job curator notes/comments on a shared list (rejected in brainstorming as unnecessary scope).
- No manual ordering of jobs within a list (sort is `added_at DESC`); no drag-to-reorder.
- No migration path from an existing shared board into a job list — a query and a job set are different data, so there is nothing meaningful to auto-convert.
- No author-label-style attribution on a shared list (unlike boards' optional `author_label`) — out of scope per the agreed design (name + description only).

## Decisions

**New package `internal/application/joblists`, not an extension of `internal/search/savedsearch` or `internal/job/collections`.** Job lists depend on `job` (jobs referenced by id) and belong to the `application` layer, same as `jobtracking` — putting them in `search` (a different block in the same layer) would violate the same-layer no-cross-import rule, and `internal/job/collections` is a different block two layers down with a completely different ownership model (code-owned registry vs. user-owned rows). A new sibling package to `jobtracking` is the only placement consistent with the layering table.

**Two tables (`job_lists`, `job_list_items`), not a discriminator column on an existing table.** Considered and rejected in brainstorming: overloading `saved_searches` (query-shaped) or `user_jobs` (one row per `(user_id, job_id)`, no grouping) both would have forced nullable branching or broken the primary key semantics those tables already carry for their existing purpose. A dedicated many-to-many pair is the direct fit for "a job belongs to zero or more named lists."

**The slug-minting helper moves to a shared `internal/dict/slugmint` package instead of being duplicated.** It has exactly one caller today (`savedsearch/share.go`) and will have exactly one caller after this change (`joblists`) — the caller is swapped, not multiplied, so this is a relocation of existing logic, not new shared infrastructure built ahead of need. It lands in `dict`, not `platform` as first considered: the helper depends on `internal/dict/normalize` for transliteration, and `platform` sits below `dict` in the layer order (a lower layer must not import a higher one), so `platform` cannot host it. `dict` sits below both `search` and `application` (this helper's past and present callers), so it is import-legal from both.

**Public read model mirrors boards' shape (bare, unauthenticated, owner-free), applied to a job list instead of a query.** `GET /api/v1/lists/:slug` returns `{name, description, jobs}` the same way `GET /api/v1/boards/:slug` returned `{name, query, author_label}` — same privacy posture (no owner id/email), same 404-on-unshared behavior, same idempotent-share-keeps-slug semantics. Jobs are rendered through the existing `jobview` projection used everywhere else, so a closed/expired job naturally carries whatever status field other surfaces already show it with — no new status representation is invented for this feature.

**Old public board links simply 404, per explicit decision.** No redirect, no grace period, no notification to affected users — agreed as acceptable since board sharing saw little use and a query cannot be mechanically turned into a job list.

**Migrations:** one new migration adds `job_lists`/`job_list_items`; a second (separate file, since migrations are add-only) drops `saved_searches.public_slug`, `saved_searches.author_label`, and `saved_searches_public_slug_idx`. Both are additive-schema-then-subtractive-schema in the normal migration sequence — no data backfill needed since board sharing is being cut off outright rather than converted.

**A 200-job-per-list cap, added during code review, not in the original design.** Unlike a saved search — whose "result set" is naturally bounded by however many jobs match a query, so `saved_searches` never needed a per-item cap — `GetPublicList` (the public, unauthenticated `GET /api/v1/lists/:slug` read) renders every job a list contains, each through a query that regexp-strips HTML from `jobs.description` (a TOASTed column) for its blurb. With no per-list bound, that cost scales with however many jobs one list accumulated, on an endpoint anyone can hit anonymously given a slug — a materially worse shape than the board-sharing path it replaces, which paged 20 results at a time through the search index. `internal/application/joblists.Service.AddJob` now enforces the cap (`ErrListFull`, mapped to `409`), exempting a re-add of an existing member so idempotency is preserved at the cap.

## Risks / Trade-offs

- **[Risk]** A user with an actively-shared board (e.g. `/b/founding-engineer-founder-yop9`, referenced from external sites/resumes) loses that link with no warning. → **Mitigation**: none planned, per explicit decision in brainstorming (low usage, no clean automated migration exists). Acceptable one-time breakage, consistent with "simply cut off" being the chosen option.
- **[Risk]** Deleting `share.go`'s slug minter while `joblists` is being built could leave a window where neither package has it. → **Mitigation**: move the helper (not delete-then-recreate) in the same PR/commit sequence as introducing `joblists`'s dependency on it, so it never lands unreferenced.

## Migration Plan

1. Ship the `job_lists`/`job_list_items` migration and the `joblists` package + API routes + frontend, additive and behind no flag (new surface, nothing depends on it yet).
2. Ship the frontend job-list UI (account section, job-card "Add to list" affordance, public `/l/:slug` page).
3. Remove board-sharing code paths (`Share`/`Unshare`/`GetPublicBoard`, `GET /boards/:slug`, `/b/[slug]`, the share/unshare UI in `SavedSearchesView.svelte`) — this step is independent of steps 1-2 and could ship in the same or a later deploy; ordering doesn't matter functionally since the two features don't interact, but doing job lists first means there's a replacement available before boards disappear.
4. Ship the migration dropping `saved_searches.public_slug`/`author_label` — after step 3's code no longer references those columns.

Rollback: each step is a normal revert of its own migration/deploy; no cross-step coupling to unwind since the two capabilities never share state.
