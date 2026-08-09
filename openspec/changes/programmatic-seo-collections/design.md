## Context

`/collections/[slug]` (`web/src/routes/collections/[slug]/+page.svelte` +
`+page.server.ts`) already unifies two collection kinds behind one route via
`collectionBySlug()` in `web/src/lib/collections.ts`:

- **Filter collections** (`FILTER_COLLECTIONS`): frontend-only, map a slug to
  a fixed set of `/jobs` facet params (`work_mode`, `regions`, `countries`,
  `skills`, `category`, `seniority`, `role`).
- **Company collections** (`COLLECTIONS`, generated from `internal/collections`
  by `cmd/gen-contracts` into `web/src/lib/generated/contracts.ts`): a Go
  registry (YC, Techstars, a16z, Big Tech, Unicorns, visa-sponsor registers)
  whose membership is propagated onto every job as `jobs.collections`, served
  by the `collections` search facet.

Both kinds' landing pages already SSR the collection's job feed and already
have the resulting `meta.total` in hand — that's the number this change
surfaces in the document metadata. A full initial code survey (subagent
research) missed the company-collection registry entirely on the first pass
and an earlier draft of this design proposed rebuilding it as a new
`/companies/collections/[slug]` page type; that was corrected once
`internal/collections` was found — this design does not touch it.

## Goals / Non-Goals

**Goals:**
- Interpolate the live, exact job count into `<title>`/`<h1>`/OG title on
  `/collections/[slug]`, for both collection kinds.
- Grow `FILTER_COLLECTIONS` with more verified high-intent combinations.
- Add a bounded "see also" block to the job detail page that links into
  existing `/collections/[slug]` pages, sourced from the viewed job's own
  facets and its `collections` field.

**Non-Goals:**
- No new page type, route, or backend endpoint. No cross-product / auto-generated
  combinations — every collection stays hand-curated with a verified live count.
- No change to `internal/collections`, its registry, or the `jobs.collections`
  propagation — this change only *reads* the field, never writes it.
- No keyword-volume-driven prioritization for the initial `FILTER_COLLECTIONS`
  growth — Search Console isn't connected for this project yet; the list is
  judgment-based on standard job-board search patterns and should be revisited
  once GSC or GA4 data is available.

## Decisions

**Exact count, not rounded.** `"1,234 React Jobs"` rather than `"500+ React
Jobs"`. Trades a snippet that can look stale between Google re-crawls for a
number that's always literally true when someone lands on the page. Product
decision, not a technical constraint — rounding is a one-line change later if
this proves noisy in Search Console.

**"See also" fill order: job facets before job collections, then popular
fallback.** Source A (role/region/skills → `FILTER_COLLECTIONS`) is more
specific to the exact posting than Source B (company membership →
`COLLECTIONS`), so A's matches are listed first. Both sources are matched
against data the job page already loads — no new fetch. When the combined
match count is below the target (~4-6), the block pads with a fixed popular-collections
fallback (`remote-worldwide` + top skill collections) so the block is never
sparse or absent, without ever inventing a link to a non-existent slug.

**No live count in the see-also block itself.** Only the block's *target*
pages compute and show counts (per the title/H1 change above); the block is a
pure link list built from two in-memory arrays plus the current job's
already-loaded facets, so it adds no request and no latency to the job page.

**Matching logic as a pure function.** The see-also selection (facets + job
collections + both registries → ordered, deduped slug list) is implemented as
a pure function independent of any HTTP call, so it's unit-testable without
mocking the job page's data loading.

## Risks / Trade-offs

- **Stale-looking snippet** → Google caches titles for days/weeks; an exact
  live count can visibly lag the cached SERP snippet. Mitigation: accepted
  trade-off (see Decisions); revisit rounding if Search Console flags it once
  connected.
- **Empty match pool for an obscure job** → the popular-collections fallback
  guarantees the block is never empty as long as at least one collection
  exists at all; if the entire fallback pool is somehow exhausted (won't
  happen with the current registry sizes), the block silently renders fewer
  than the target rather than fabricating a link.
- **`FILTER_COLLECTIONS` growth reintroducing thin pages** → mitigated by the
  same per-entry live-count verification the existing entries already require,
  carried into implementation as a manual/scripted check before each new entry
  ships (see proposal's Impact).

## Migration Plan

No data migration. Frontend-only change, deployed with the normal release
process. No feature flag — the count-in-title change and the see-also block
are both additive and low-risk (worst case: a wrong or missing count, or a
short link list), not worth gating behind a flag per project convention
(MVP-stage, no infra before there's a concrete need).

## Open Questions

None outstanding — see the proposal's Non-Goals for deliberately deferred
scope (keyword-volume-driven list prioritization, once Search Console is
connected).
