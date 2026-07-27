## Why

When the browser extension cannot recognise the page the user is on, the panel falls back
to matching scraped text — a card with no company, no salary, no skills, and none of the
catalog's enrichment. The user is looking at a real vacancy; we just do not carry it.

Everything needed to carry it already exists. `internal/linksource` resolves a single
job-detail URL into a fully parsed vacancy under the destination's own identity — per-ATS
adapters (greenhouse, ashby, lever, workable, habrcareer, geekjob, remoteyeah, bairesdev)
plus a last-resort resolver for any page carrying a `schema.org/JobPosting` ld+json block.
`cmd/resolve-url` already drives that registry into the canonical `UpsertJob` write path,
enrichment enqueue and search index included. It is an operator tool: the only way to use
it is to be an operator with a shell.

So a user who finds a vacancy we lack has no way to hand it to us, and the crowdsourced
path we do expose (`POST /me/contributions`) contributes a *board*, not a vacancy — for an
aggregator page it records a link for manual triage and nothing appears in the catalog.

## What Changes

- A new authenticated endpoint `POST /api/v1/jobs/resolve` takes a page URL and answers
  with the catalog posting for it, importing the posting when we can read the page:
  1. **Already ours** — the URL resolves to a catalog posting (the same two tiers as
     `/jobs/find`): answer with its slug, import nothing. This is what keeps the endpoint
     from minting duplicates of postings we already carry.
  2. **Importable** — a `linksource` adapter (or the generic JobPosting resolver) parses
     the page: upsert it through the canonical write path and answer with the new slug.
  3. **Unreadable** — no adapter can parse it: record the link through the existing
     contribution service, which triages a recognised novel board as `pending` (rewarded)
     and anything else as `review`, and say so.
- The resolve-and-write half of `cmd/resolve-url` moves into a package both the command
  and the handler call, so there is one definition of "import a vacancy from a link" —
  registry order, write path, enrichment enqueue and index push included.
- The endpoint shares the existing per-user contribution rate limiter: it makes the server
  fetch a user-supplied URL, exactly the amplifier that limiter exists for. Outbound
  fetches go through `sources.NewClient`, which dials via the `safehttp` SSRF guard.

## Capabilities

### New Capabilities
- `posting-import-by-url`: importing the vacancy on a user-supplied page into the catalog
  when we can parse it, and routing the page to manual triage when we cannot.

### Modified Capabilities
- `posting-url-resolution`: the two-tier resolve becomes callable from more than the find
  endpoint (no behavior change to `/jobs/find` itself).

## Impact

- `internal/linkimport/` — new: resolve a URL through the linksource registry and write the
  vacancy (lifted from `cmd/resolve-url`, unchanged in behavior).
- `cmd/resolve-url/` — becomes a thin CLI over that package.
- `internal/handler/` — new `ResolveJob` handler, route wired with `keyAuth` and the
  shared contribution limiter; the `/jobs/find` tiers extracted into a reusable lookup.
- No migration; no new table (contributions and jobs both already exist).

## Non-Goals

- **No credits for an imported vacancy.** The contribution reward is scoped to a novel
  board, which is a lastingly larger contribution than one posting; changing that economy
  is a product decision, not this change.
- **No moderation queue for imports.** A page that parses into a JobPosting is written
  straight to the catalog, exactly as `cmd/resolve-url` writes it today; `job_submissions`
  remains the hand-reviewed path for a vacancy typed in by hand.
