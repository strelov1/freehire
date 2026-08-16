## 1. Listing crawl

- [x] 1.1 Adapter skeleton: `seek` type over a `JSONGetter`+`JSONPoster` transport role, `Provider()`
      returning `"seek"`, `NewSeek` constructor, and the market table mapping region `au`/`nz` to its
      host, site key, search scope and locale. Test: an entry with an unknown region fails the board
      with an error naming it; a known region builds a search URL carrying host, site key, scope,
      subclassification, page, page size and newest-first sort.
- [x] 1.2 Listing walk: page until a page adds no new posting id, backstopped by a page ceiling.
      Test: a second page with fresh ids is walked; a page repeating the first page's ids ends the
      walk; the response struct carries no `totalCount` field to reach for.
- [x] 1.3 First-page-vs-later-page failure rule. Test: a failing first page returns a board-level
      error; a failing second page returns the first page's postings with no error.
- [x] 1.4 `Fetch` maps a listing posting to a `Job`: id, title, `/job/<id>` URL on the market host,
      free-text location, structured country from the posting's country code, and listing date.

## 2. Posting mapping

- [x] 2.1 Employer resolution: profiled employer name, falling back to the advertiser name; a
      posting whose only name is the `"Private Advertiser"` placeholder, or which has no name at
      all, is dropped. Test: all three cases plus a posting with no id.
- [x] 2.2 Work-mode mapping from SEEK's work arrangements, preferring the most remote arrangement;
      unstated leaves it empty and `Remote` false. Test: remote / hybrid / on-site / unstated, and
      a posting offering several.
- [x] 2.3 Employment-type mapping from SEEK's work types into `vocab.EmploymentTypeValues`; an
      unmapped type leaves it empty. Test: full time, part time, contract/temp, unmapped.
- [x] 2.4 Salary label folded into the description as a leading paragraph, structured salary fields
      left unset; no label yields no paragraph. Test: both cases.

## 3. Description hydration

- [x] 3.1 `FetchNew` implementing `HydratingSource`: hydrate only postings the `seen` predicate
      reports as new, mark the rest `SeenRefresh`. Test: exactly the new posting's id reaches the
      detail transport; the seen posting carries `SeenRefresh` and no body.
- [x] 3.2 GraphQL `jobDetails` detail call, response sanitized through `sanitizeHTML`; a failed
      request or an empty body falls back to the list-only job rather than dropping the posting.
      Test: success appends the body after the salary paragraph; transport error and empty content
      both keep the posting.

## 4. Registration and configuration

- [x] 4.1 Markers: `aggregator` (not `boardless`) and `sweepGrace` of 14 days. Test: the adapter
      appears in `AggregatorProviders`, is absent from the boardless-driven exclusions, and reports
      its window through `SweepGraceWindows`.
- [x] 4.2 Register `NewSeek(c)` in `sources.All`, keyless.
- [x] 4.3 Write `sources/seek.yml` — 22 AU and 21 NZ entries with live-verified counts — and confirm
      `go run ./cmd/validate-sources` passes.

## 5. Verification and documentation

- [x] 5.1 Live smoke run against both markets: `go run ./cmd/ingest sources/seek.yml` reaches SEEK,
      yields postings with employers and hydrated descriptions, and reports no board failures.
- [x] 5.2 Record the platform's verified traps in `internal/sources/AGENTS.md`: the interstitial
      covers pages but not the API, `totalCount` varies with `pageSize`, the ~550 result window,
      `where` is load-bearing, and the `"Private Advertiser"` placeholder.
