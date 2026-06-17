## Context

`internal/sources` is a registry of single-purpose adapters, each implementing `Source`
(`Provider()` + `Fetch(ctx, CompanyEntry) []Job`). Board-based adapters whose list endpoint
omits the description (`oracle`, `icims`, smartrecruiters, …) page a list, then fetch each
posting's detail concurrently through the shared `fetchDetails(postings, defaultDetailWorkers,
fetch)` helper. `apply.careers.microsoft.com` runs on **Eightfold AI**, which exposes two
public GET-JSON endpoints reached over the shared `HTTPClient.GetJSON`:

- `GET https://<host>/api/pcsx/search?domain=<domain>&query=&start=<n>&num=10&sort_by=relevance`
  → `{ "data": { "positions": [ … ], "count": <int> } }`. Each position carries `id`,
  `displayJobId`, `name`, `locations[]`, `postedTs` (Unix sec), `workLocationOption`, and
  `atsJobId` — **but no description**.
- `GET https://<host>/api/apply/v2/jobs/<id>?domain=<domain>` → the full posting, including
  `job_description` (HTML) and `canonicalPositionUrl`.

This is the same list+detail shape as `oracle`, differing only in the endpoint specifics.

## Goals / Non-Goals

**Goals:**
- A reusable `eightfold` board-based adapter that yields the normalized `Job` shape for a
  tenant's whole catalogue.
- Page the `/api/pcsx/search` list and fetch each position's detail for the description.
- A fixture-backed unit test that pins the list and detail JSON shapes and the mapping.

**Non-Goals:**
- No headless browser, proxy, or auth (both endpoints are public GET JSON).
- No new env vars, DB columns, or migrations.
- No search-by-query/work-site filtering — the crawl walks the full catalogue (empty query),
  which is what ingest needs.

## Decisions

- **Board id is `"host/domain"`, parsed like `oracle`'s `"host/site"`.** Eightfold needs the
  public host for request paths AND a required `domain` query parameter (the tenant key, e.g.
  `microsoft.com`), and the two are not reliably derivable from each other (a tenant's `domain`
  need not equal the host's registrable domain). So the board carries both explicitly;
  `parseEightfoldBoard` splits on the first `/` and rejects a board missing either half. This
  reuses the established two-part board convention rather than guessing a domain from the host.
- **Two list-API generations, auto-detected.** Eightfold tenants run one of two list APIs: the
  newer `/api/pcsx/search` (positions under `data`, `postedTs`, `workLocationOption`; e.g.
  Microsoft, Micron) or the legacy `/api/apply/v2/jobs` (top-level positions/count, `t_create`,
  `work_location_option`, and a list-level `canonicalPositionUrl`; e.g. Netflix). A tenant
  supports exactly one — the other returns `403` — so `listPositions` tries pcsx first and, on
  any error, falls back to the v2 list (restarting from the first page). This keeps the board id
  uniform (`host/domain`) for every tenant, which matters for bulk-adding discovered tenants
  without knowing each one's generation; the cost is one wasted (fast, 403) request per legacy
  board per crawl. One `eightfoldPosition` struct decodes both shapes (both field-name variants
  as separate tags, unused ones stay zero); the detail endpoint `/api/apply/v2/jobs/<id>` is
  shared, so only the list URL + envelope differ. `posted_at` takes `postedTs` else `t_create`;
  `location` takes the v2 single-string `location` else the first of `locations[]`; `work_mode`
  takes whichever work-option field is set.
- **Page by `start`; the server caps `num` at 10.** Requesting `num=200` still returns 10
  positions, so the page size is fixed at 10 and `start` is the only lever. The loop advances
  `start` by the number of positions actually returned and stops when a page is empty OR the
  running count reaches `data.count`. The empty-page check is the correctness backstop if
  `count` is ever wrong/absent; `count` is the optimization, mirroring `oracle`/`jibe`.
- **Detail fetch via the shared `fetchDetails`.** The list omits the description, so each
  position maps through `detail()` which GETs `/api/apply/v2/jobs/<id>?domain=<domain>`. A
  failed detail request returns `ok=false` so that one posting is dropped without aborting the
  board — the standard isolation `oracle`/`icims` use, with `defaultDetailWorkers` concurrency.
- **List carries the metadata, detail carries the description.** `external_id`, `title`,
  `location`, `posted_at`, and `work_mode` come from the list position (the detail's
  `work_location_option` was observed null while the list's `workLocationOption` is populated);
  only `description` and the canonical `url` come from the detail. This keeps each request's
  role explicit and avoids re-reading list fields from the detail.
- **`url` is the detail's `canonicalPositionUrl`, with a deterministic fallback.** When the
  detail omits it, build `https://<host>/careers/job/<id>` (the public position page the
  listing's relative `positionUrl` points at).
- **`external_id` is the numeric position `id` (stringified).** It keys the detail endpoint and
  the canonical URL; `displayJobId`/`atsJobId` are human-facing labels, not the API key.
- **Work mode reuses `workplaceTypeMode`.** Eightfold's `workLocationOption` is already
  `remote`/`hybrid`/`onsite` (lowercase), which the existing helper maps to our vocabulary; an
  unknown/empty value yields `""` and the pipeline falls back to the location string. No new
  mapping function is warranted.
- **Dates via `parseEpochSeconds`.** `postedTs` is a Unix-second timestamp; reuse the existing
  helper so `NotFuture` guarding stays consistent with the other epoch-dated adapters.

## Risks / Trade-offs

- **N+1 request cost.** One detail GET per posting (Microsoft's tenant lists ~1.4k positions),
  the same cost profile as `oracle`/`icims`; `fetchDetails` bounds the fan-out and isolates
  failures. Accepted: the list has no description, so there is no cheaper path.
- **`domain` is a hard requirement.** Omitting it returns `422`. The `"host/domain"` board
  format makes it explicit and config validation already requires a non-empty board for a
  board-based provider; `parseEightfoldBoard` fails fast on a malformed half.
- **Brittle to an Eightfold API rename.** If the `/api/pcsx/search` or `/api/apply/v2/jobs`
  shape changes, the mapping breaks — guarded by the committed fixtures + unit test, the same
  posture as every other JSON adapter.
