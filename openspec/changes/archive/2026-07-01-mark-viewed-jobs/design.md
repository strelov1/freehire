## Context

Per-user view tracking already exists: opening a job detail records a `user_jobs`
row (`viewed_at NOT NULL DEFAULT now()`), and `/api/v1/me/jobs` can read it back.
But the browse list (`ListJobs`) and search (`SearchJobs`) are deliberately
public and unauthenticated, and the `jobview.Job` wire shape carries no per-user
state — so a signed-in user cannot see which postings they already opened.

Two architectural constraints shape the solution:
- **"Public job reads stay unauthenticated"** is a standing project convention.
  We do not want to make `ListJobs`/`SearchJobs` auth-aware.
- **Search runs through Meilisearch, not Postgres.** A per-job `viewed` flag
  joined into the read path would not reach search hits without a separate
  cross-reference anyway.

## Goals / Non-Goals

**Goals:**
- A signed-in user can tell, at a glance, which jobs in the list and in search
  results they have already opened.
- Keep the public read path unauthenticated and the `jobview.Job` wire shape
  (and generated TS contracts) unchanged.
- Reuse the existing server-side `user_jobs` data so the marking is cross-device
  and reflects history recorded before this feature shipped.

**Non-Goals:**
- No per-job `viewed` field on the public job shape.
- No SSR of viewed state (no extra request on every page load).
- No new "mark unviewed" / clear-history affordance.

## Decisions

**Decision: ship viewed state as a separate slug-set endpoint, cross-referenced
client-side.** A new `GET /api/v1/me/jobs/viewed` returns the set of
`public_slug`s the caller has viewed (`{"data": [slug, ...]}`), guarded by
`RequireAuthOrKey` like the other `/me` reads. The SPA loads it once when a
signed-in user opens the browse view, holds it in a small reactive store, and a
job card dims when its slug is in the set.
- *Why over an auth-aware list:* keeps the public list/search endpoints
  untouched (honours the convention) and works identically for Postgres-backed
  list results and Meilisearch-backed search results, since the match is a pure
  client-side set lookup on `public_slug`.
- *Why over localStorage-only:* the source of truth is already server-side
  (`user_jobs`), so a slug set is cross-device and includes pre-feature history;
  a localStorage cache would silently diverge from it.

**Decision: dim the whole card via opacity, restore on hover.** `JobRow` gets
`class:opacity-60={isViewed}` plus `hover:opacity-100`, so a viewed card reads as
"already seen" but returns to full strength on hover to signal it is still
clickable. A `dimViewed` prop (default `true`) lets the My Jobs surfaces
(History tab, board) opt out — there every card is viewed, so dimming all of them
would be noise.

**Decision: unbounded slug list, no pagination.** The query is keyed on the
`user_jobs` primary key `(user_id, job_id)` and returns compact slug strings;
returning the full set is the simplest correct behaviour. If a power user's set
ever grows large enough to matter, pagination is the documented seam.

## Risks / Trade-offs

- **Hydration flash** → The first SSR-rendered page paints before the viewed-slug
  fetch resolves, so viewed cards dim ~100ms after hydration. Accepted; avoids an
  extra blocking request on every page load. Mitigated for the common case
  (opening a job then going back) by `markViewed(slug)` updating the store
  locally on view-record.
- **Growing slug set** → For a heavy user the payload grows unbounded. Low risk
  at current scale (compact strings, one cheap indexed query); pagination is the
  noted seam if it ever bites.
- **Stale set within a session** → The set is loaded once per browse view, so a
  job viewed in another tab is not reflected until reload. Acceptable for a
  passive convenience cue; `markViewed` keeps the current tab consistent.
