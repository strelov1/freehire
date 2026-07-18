## Why

NoFluffJobs (nofluffjobs.com) is a major Polish/CEE IT job board — the complement to justjoin.it,
which we already crawl. A spike VALIDATED that its `/api/posting` listing is a single keyless JSON
document enumerating every open posting with structured facets (technology, seniority, category,
salary, location, remote flag), and that a keyless per-posting `/api/posting/<slug>` detail carries
the full description. Adding it widens Polish/CEE IT coverage through the same hydrating-adapter
pattern justjoin already uses.

## What Changes

- Add a `nofluffjobs` source adapter that streams the `https://nofluffjobs.com/api/posting` listing
  (a single ~60 MB JSON document — fetched via `GetStream`, past the size-capped `GetJSON`) and maps
  each posting to a normalized `Job`: `id` → `ExternalID`, `https://nofluffjobs.com/job/<url>` →
  canonical URL, `title`, `name` → company, `location.places[0]` / `fullyRemote` → location + remote,
  `posted` (epoch ms) → posted-at, `technology` → skills (via the skilltag dictionary), and
  `seniority[0]` → seniority (mapped into freehire's vocabulary).
- **Hydrating adapter (justjoin pattern).** Implement `FetchNew(ctx, entry, seen)`: the listing has
  no description, so a posting's description is fetched from `/api/posting/<url>` (assembling
  `details.description` + `requirements.description`) only when it is **new** (not in the seen set);
  already-ingested postings refresh liveness from the list without a detail request. Plain `Fetch`
  is the list-only fallback. This bounds the per-run detail fan-out to genuinely new postings.
- The adapter is **boardless** (one global API, no per-tenant board) and an **aggregator** (company
  per posting), so it stays in the source facet — exactly like justjoin.
- Drop a posting with no `id`, no `url` (no canonical URL), or no company (empty company breaks the
  slug).
- Register `nofluffjobs` in `sources.All` and add `sources/nofluffjobs.yml` with a single boardless
  entry, crawled by its own `cmd/ingest` cron schedule.

## Capabilities

### New Capabilities
- `nofluffjobs-source`: the `nofluffjobs` adapter — its streamed listing crawl, posting→`Job` mapping
  with structured facets, hydrating `FetchNew` detail description, drop rules, and boardless-aggregator
  classification.

### Modified Capabilities
<!-- None. Boardless hydrating aggregator; inherits the standard ingest sweep, seen-set hydration,
     aggregator dedup, and board-health machinery unchanged. -->

## Impact

- **New code:** `internal/sources/nofluffjobs.go` (+ `_test.go`); `sources/nofluffjobs.yml`.
- **Touched code:** one line in `sources.All` (registry).
- **Ops:** a new `cmd/ingest sources/nofluffjobs.yml` cron schedule (deploy-time, in freehire-ops).
- **Cost:** first run hydrates all ~8 400 postings (one detail request each); steady-state runs
  hydrate only new postings. If the site rate-limits the prod IP, enroll in `proxiedProviders`
  (a config flip; noted as a seam, not built now).
- **No migrations, no API changes, no new dependencies, no key (keyless).**
