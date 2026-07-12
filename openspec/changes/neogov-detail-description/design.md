## Context

`internal/sources/neogov.go` is a list-only adapter: it parses the listing fragment's
`li.list-item` cards and stores the card's `.list-entry` teaser as the description. A
spike against a live posting (`governmentjobs.com/careers/louisiana/jobs/5382055`)
confirmed the full body is server-rendered on the detail page inside
`<div id="details-info" class="tab-pane active fr-view">` as a `<dl>/<dt>/<dd>`
structure (~4KB text), reachable by a plain GET with **no** `X-Requested-With` header.
The package already provides `innerHTML(node)`, `sanitizeHTML(s)`, `firstByClass`,
`walk`, and `attr`; the `neogovHTTP` port already exposes
`GetTextWithHeaders(ctx, url, headers)`.

## Goals / Non-Goals

**Goals:**
- Store the full NEOGOV posting body as sanitized HTML, replacing the teaser snippet.
- Keep one board's detail fetches bounded and resilient — a per-card failure degrades
  to the snippet, never a blank description or a dropped job.
- Backfill existing rows through the normal ingest path, no new worker.

**Non-Goals:**
- Parsing the detail page's Benefits/Questions tabs (`#details-benefits`,
  `#details-questions`) — only `#details-info` is the job description.
- Extracting the absolute posted date from the detail page (a separate concern the
  list-only comment already flagged; out of scope here).
- Any schema, API, migration, or env change.

## Decisions

**Fetch the detail page per card, reuse the existing port.** `Fetch` already builds
each card's absolute detail URL. After parsing the listing, the adapter fetches each
card's detail via `GetTextWithHeaders(ctx, url, nil)` — the same port, but with a nil
header map because the detail page is plain server-rendered HTML. Alternative
considered: extend the port with a header-less `GetText`. Rejected — passing `nil`
headers to the existing method is the minimal change and the shared HTTP client
already tolerates it.

**Extract `#details-info` by its stable class, sanitize its inner HTML.** The
container carries `class="... fr-view"` (Froala editor view) and `id="details-info"`.
The parse locates it and yields `sanitizeHTML(innerHTML(node))`, matching the
sanitized-HTML description contract every other detail adapter follows (tbank,
dataart, careerspage). The listing snippet is retained as the degrade fallback.
A small `firstByID` helper (mirroring `firstByClass`) selects the container by id for
precision; matching `fr-view` by class is the fallback if id proves unstable across
tenants.

**Bound the detail fetches.** The per-board detail requests run under a bounded
concurrency (a small worker pool over the parsed cards), satisfying the source-ingest
"detail requests SHALL be bounded" requirement and matching the N+1 shape of other
detail adapters. Sequential is acceptable for correctness; bounded-concurrent keeps a
large agency's crawl within the cron window.

**No backfill script.** `UpsertJob`'s conflict clause is
`description = COALESCE(NULLIF(EXCLUDED.description, ''), jobs.description)` — a
non-empty incoming description overwrites the stored one — and `content_hash` includes
the description (`jobhash.Of`), so a re-crawl reports the row `changed` and re-pushes
it to the search index. The next `cmd/ingest sources/neogov.yml` run therefore
corrects every existing NEOGOV row in place. Alternative considered: a one-off
`cmd/backfill-neogov`. Rejected as redundant with the self-healing re-ingest path
(the same pattern as job-reality-signal and incremental indexing) — an operator runs
the normal crawl once post-deploy for an immediate backfill.

## Risks / Trade-offs

- **Detail markup varies across tenants** → extraction keys on `#details-info` /
  `.fr-view`, both NEOGOV platform chrome (not per-agency content), and degrades to
  the snippet when absent, so an unexpected layout yields the old behavior, not a
  blank.
- **N+1 request volume on large agencies** → bounded concurrency plus the existing
  `board_health` cooldown backoff keep a slow or failing board from being hammered;
  detail failures are per-card and non-fatal to the board crawl.
- **Sanitized HTML now where plain text was** → correct per the source-ingest
  sanitized-HTML requirement; the frontend already renders adapter descriptions as
  sanitized HTML, so this aligns NEOGOV with every other detail adapter.
