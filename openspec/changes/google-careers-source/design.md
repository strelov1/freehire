## Context

`internal/sources` is a registry of single-purpose adapters, each implementing `Source`
(`Provider()` + `Fetch(ctx, CompanyEntry) []Job`). Boardless single-company adapters
(`uber`, `amazon`) crawl one company's whole catalogue with no per-tenant board id. Google's
careers site has no JSON API anymore, but its list pages are server-rendered: the full job
payload is inlined in an `AF_initDataCallback({key:'ds:1', data:[…]})` `<script>`. The shared
`HTTPClient` exposes `GetHTML(ctx, url) (*html.Node, …)`, and `html.go` provides a `walk`
DOM traversal and `textContent`. `sanitizeHTML` cleans description HTML. This is the same
shape as the existing server-rendered adapters (successfactors, vk), differing only in how
the data is located on the page (an embedded JSON blob rather than schema.org microdata).

## Goals / Non-Goals

**Goals:**
- A `google` boardless adapter that yields the normalized `Job` shape for the whole catalogue.
- Extract jobs from the embedded `ds:1` payload with no per-job detail fetch.
- A fixture-backed unit test that pins the brittle positional mapping.

**Non-Goals:**
- No headless browser, proxy, or auth.
- No new env vars, DB columns, or migrations.
- No search-by-query support — the crawl walks the full catalogue (empty query), which is
  what ingest needs.

## Decisions

- **Locate the blob, then parse JSON — don't parse the array out of raw HTML by regex on the
  whole page.** Use `GetHTML` + `walk` to find the `<script>` text node containing
  `AF_initDataCallback({key: 'ds:1'`, take its `textContent`, then a single regex
  (`data:(\[…\]), sideChannel`) isolates the JSON array, which `json.Unmarshal`s into a
  `[]json.RawMessage` tree. Locating via the DOM (not a page-wide regex) keeps the extraction
  robust to unrelated markup and matches how the other HTML adapters work.
- **Model the record positionally with a typed accessor, not anonymous indexing scattered
  through the mapper.** Unmarshal each job record into `[]json.RawMessage` and read fields by
  index through small helpers (`recString(rec, 1)` → title) so the brittle positions live in
  one documented place. Each index gets a `// [n] = …` comment. This is the single point a
  fixture test guards.
- **Stop condition mirrors `uber`.** Loop `page := 1; ; page++`: unmarshal `data[0]` (job
  list) and `data[3]` (total count). Break when `data[0]` is empty/null OR the running count
  reaches the total. A page past the end yields a null `ds:1` job list (verified live), which
  the empty check already covers.
- **`url` is the public results page, not the apply link.** Live probing showed
  `…/jobs/results/<id>` (id only, no slug) returns 200 and serves the posting — Google ignores
  the trailing slug entirely (even a wrong slug resolves). So the adapter builds the bare
  `…/jobs/results/<id>` URL (like uber's id-only URL), avoiding any dependency on reproducing
  Google's slug. The embedded apply URL `[2]` is sign-in-gated and unstable, so it is not used.
- **Description assembly.** Concatenate the "about the job" body, responsibilities, and
  qualifications HTML fragments (record positions `[10]`, `[3]`, `[4]`) in that reading order,
  then `sanitizeHTML` the whole. Each fragment is already an HTML string.
- **Dates.** The record carries Unix-epoch timestamps; reuse `parseEpochSeconds` (already
  present, used by gem) so future-date guarding (`NotFuture`) stays consistent.
- **Location + country.** The locations array element is `["City", […], null,null,null,
  "CC"]`; join the city name(s) into the free-text `location` string the pipeline already
  parses for geography. The ISO country code is a nice-to-have the location dictionary will
  re-derive, so the adapter keeps emitting free-text `location` (no new structured field) to
  stay consistent with every other adapter and avoid a pipeline change.

## Risks / Trade-offs

- **Brittle positional layout.** If Google reorders the `ds:1` record, the mapping silently
  mis-maps. Mitigation: a committed HTML fixture + a unit test asserting the mapped fields;
  the test fails loudly when the layout drifts (same posture as amazon's `id_icims`-as-string
  catch). Accepted: there is no stable keyed API to target.
- **Page-size / total-count coupling.** The stop condition trusts `data[3]` (total) and the
  empty-page check. If Google drops the total field, the empty-page check still terminates the
  loop, so the crawl is safe either way; the total is an optimization, not a correctness
  requirement.
- **No work-mode signal.** The payload has no structured remote flag, so `WorkMode` is left
  empty and the pipeline's location-string heuristic applies — identical to uber/amazon.
