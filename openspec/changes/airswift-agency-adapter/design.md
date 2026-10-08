## Context

Full batch rationale: `docs/superpowers/specs/2026-10-08-oilgas-agency-adapters-design.md`.
Verified live (2026-10-08):

- `https://www.airswift.com/sitemap.xml` is a flat `<urlset>`, ~2930 `<loc>` entries,
  ~1841 matching `/jobs/<slug>-<id>`.
- A live posting's `JobPosting` ld+json carries `title`, HTML `description`,
  `datePosted`, `validThrough`, `employmentType`, `hiringOrganization` (`name` is the
  literal string `"Airswift"` on every posting checked — not a real end client), and
  `jobLocation` (array of `Place`, address/city/region/country — same shape
  `dataart.go` already handles).
- An expired posting returns `HTTP 200`, no `JobPosting` block, and instead a
  `<div class="c-jobs-article-expired ...">` containing the text "Thank you for your
  interest in this role, but we are no longer accepting applicants." This is a stable,
  presentation-level marker (a CSS class), not a flaky text match.

## Goals / Non-Goals

**Goals:**
- Enumerate the sitemap, map each live posting's ld+json to a `Job`.
- Drop expired postings outright (same category as a 404) rather than storing an
  `unreadableDetail` stub for ~55-60% of the catalogue every run.

**Non-Goals:**
- Any company resolution beyond the constant `"Airswift"` — confirmed not an
  aggregator.
- Handling a genuinely missing/malformed `JobPosting` on a NON-expired page any
  differently from the existing `unreadableDetail` convention — that case is still
  ambiguous (unlike a confirmed-expired page) and gets the normal stub.

## Decisions

**Expired detection: walk the parsed tree for `c-jobs-article-expired`, not a raw-text
substring match on the HTML body.** The adapter already parses the page into an
`*html.Node` tree for `ldJobPosting`; reusing `walk`/`Attr` (`internal/ingest/sources/html.go`)
to check any node's `class` attribute for that token is more robust than a raw string
search (immune to whitespace/attribute-order changes) and costs nothing extra — the
tree is already built.

**Detail function's decision order:**
1. No id extractable from URL → drop (`false`), same as every sitemap-enumerating
   adapter.
2. Fetch error → `detailUnreadable(err)` check, same as `bayt.go`/`energyjobline.go`
   (a confirmed-gone 404/410 drops; anything else is `unreadableDetail`).
3. Page fetched OK, `c-jobs-article-expired` marker present → **drop** (`false`). This
   is new relative to `dataart.go`/`energyjobline.go`: those two treat "page fetched OK
   but no JobPosting" as `unreadableDetail` uniformly, because neither has a
   confirmed-state signal to distinguish "genuinely broken" from "this one's just
   closed." Airswift does have that signal, so it uses it.
4. No expired marker, but also no `JobPosting` block → `unreadableDetail` (the
   `dataart.go`/`energyjobline.go` default — something unexpected, not a confirmed
   closure).
5. `JobPosting` present → map normally.

**Why this matters for the stale-sweep contract.** Marking ~55-60% of every crawl as
`unreadableDetail` would (per `fetchDetails`'/`Unreadable`'s own documented contract)
withhold the stale-job close for the whole board on every single run — the same
"withholding this run's stale-job close" message the first energyjobline run logged for
being 99.99% unreadable, except here it would be a PERMANENT steady state rather than a
one-off bug, silently defeating the close sweep forever. Dropping confirmed-expired
postings outright keeps the unreadable rate at its true (low, exceptional) level.

## Risks / Trade-offs

- **[The expired-marker class name could change if Airswift re-themes the site]** →
  accepted; if it ever stops matching, the symptom is a return to the
  energyjobline-shaped "nearly everything is Unreadable" signal, which is already loud
  and already monitored the same way.
- **[A false-positive expired match on a live posting would silently drop a real job]**
  → independently re-checked post-review (2026-10-08): the literal string
  `c-jobs-article-expired` is absent (grep count 0) from the raw HTML of two different
  live postings (`project-manager-1281250`, `hull-structura-lead-1280438`) that each
  carry a working `JobPosting` ld+json block. The marker is genuinely absent from the
  DOM on live postings, not merely hidden by CSS — confirmed from the raw response body,
  not just a rendered view.

## Migration Plan

1. Ship adapter + registry line + tests through normal review.
2. After merge and deploy, run a manual crawl and sanity-check the job count (expect
   roughly 40-45% of ~1841, i.e. ~750-850 live postings, matching recon's own estimate)
   before adding the live board.
3. `cmd/add-board --provider=airswift --company='Airswift' --apply`.
4. Rollback: remove the registry line, retire the board.

## Open Questions

None — the expired-marker shape was captured directly from a live expired posting.
