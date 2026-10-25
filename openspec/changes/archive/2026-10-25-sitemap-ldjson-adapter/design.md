## Context

`internal/ingest/sources` already decodes sitemaps (`sitemap.go`), extracts a schema.org `JobPosting` (`jsonld.go`), and maps employment type, address, and remote hints (`schema.go`). `internal/ingest/linksource` turns one such page into a job, but only for `cmd/resolve-url`. Issue #3565 asks for the same parse on a whole sitemap, one board per site, added with `cmd/add-board`.

The board catalog has no URL-pattern column. `CompanyEntry.Region` is already copied through and is not part of `external_id`.

## Goals / Non-Goals

**Goals:**

- One provider, `sitemapldjson`, whose board is an absolute http(s) sitemap URL.
- Walk a urlset or a sitemap index, including a `.xml.gz` body whose bytes are gzip (magic `1f 8b`), not only `Content-Encoding: gzip`.
- Filter page URLs with the board's region string. Hydrate only new pages, up to a fixed cap, and emit already-stored URLs as `SeenRefresh` without fetching them.
- Skip pages that have no `JobPosting`. Fail the board when a child sitemap cannot be read.

**Non-Goals:**

- A JSON config file, an env list of sitemaps, a fetches-per-second knob, or a retention window.
- Reading `robots.txt` `Crawl-delay`.
- `fullBoardListing`. A partial sitemap must not look like a complete catalogue.
- Mapping `baseSalary`. The posting shape varies, and a typed field would drop the whole page when the value is an object. Enrichment fills salary.
- Treating a government portal as an aggregator. The intended board is one employer's career site.

## Decisions

### The board is the sitemap URL; the region is the URL filter

The catalog key is `(provider, board, region)`, and `externalid.Namespace` uses the board only. Putting the glob in the board string would mint a new external id every time an operator tightens the filter, so the same page would be stored twice. The region holds the pattern: empty matches every loc, a pattern with no `*` is a substring, a pattern with `*` is a glob (`*` matches across `/`).

Two boards that share one sitemap URL and differ only in region share external ids and collide on `(source, external_id)`. Operators add each sitemap once.

### Hydrating, and a failed child fails the board

The pipeline prefers `FetchNew`. A seen id is returned as `Job{SeenRefresh: true}` with no HTML fetch, so the unseen sweep does not close it. Only new URLs are fetched, shuffled, then cut at 200 per board per run, with 2 detail workers. A fetch error on a new URL is logged and omitted, so the next run retries it. A page with no `JobPosting` is the same omission, because this sitemap lists ordinary pages as well as jobs. That is the opposite of EnergyJobline, where every sitemap URL is expected to be a job and a miss is `unreadableDetail`.

An index walk stops at 3 levels and 1000 sitemap documents. A child that fails, or a body that is not a `urlset` or `sitemapindex`, returns an error. An empty valid urlset is a real empty catalogue. The adapter does not claim `fullBoardListing`: a short success after a failed child would let the company-scoped sweep close jobs that lived in the unread child.

### Gzip is a body, not a header

`GetLargeText` already undoes `Content-Encoding: gzip`. The new case is a file whose bytes start with the gzip magic and were not wrapped by that header. `decodeSitemapBody` gunzips those bytes with a 32 MiB uncompressed cap, trims space and a UTF-8 BOM, and rejects any other XML root. Shared `getSitemap` is unchanged.

### Identity and fields

The fetch URL is the resolved loc, query included. The external id is that URL with query and fragment removed and a trailing slash trimmed, except for path `/`. Relative locs are resolved against the sitemap URL. Non-http(s) locs are dropped.

`hiringOrganization` name wins when present; otherwise `CompanyEntry.Company`. A missing title or company drops the page. `TELECOMMUTE` sets remote and `workModeFromRemote`. Location and countries come from `jobLocation`, then `applicantLocationRequirements`. `jobLocation` and `hiringOrganization` unmarshal leniently so one bad field does not drop the page.

Provider kind is ATS: the page URL is the employer's own. Marking it an aggregator would let dedup suppress the only copy.

### Display name

The provider key is `sitemapldjson`. Title-casing that slug is a bad label, so `SOURCE_LABELS` says `Sitemap JSON-LD`. There is no platform domain, so no `SOURCE_LOGO_DOMAINS` entry. `SOURCE_VALUES` is regenerated from `FilterableProviders()`.

## Risks / Trade-offs

- [An empty region on a webshop sitemap spends 200 detail fetches every run on non-job URLs, forever, because a skip is not remembered] → Operators set `--region`. The adapter does not invent a negative cache.
- [The same cap also defers stored rows the seen-set withholds because they still have no body. Those rows are not emitted, so a company-scoped sweep can close them if the backlog outlasts the grace window. Already-hydrated rows are refreshed and are not in the cap.] → The run logs how many new pages it deferred, the same way EnergyJobline does. It does not emit `SeenRefresh` for a page it did not fetch: that flag only refreshes a row the catalogue already holds, and a body-less write is what the hydrating seen-set exists to avoid.
- [200 new pages and 2 workers are a judgment, not a measurement of any host] → The cap is a constant. Raising it is a code change after a real crawl is watched.
- [A government portal's URLs are the portal's, not each employer's] → KindATS is right for the company-site case this adapter is for. A multi-employer portal that needs aggregator dedup is a different provider.
- [The same sitemap added twice under two regions shares external ids] → Documented above. Do not "fix" it by moving the pattern into the board string.

## Migration Plan

Deploy the binary. Add a board with `cmd/add-board --provider=sitemapldjson --board=<sitemap URL> --region=<glob> --company=<name> --apply`. The host timer generator picks the provider up once a live board exists. Rollback is removing the board; stored rows close on the next unseen sweep of that board.

## Open Questions

None. The shape is the one accepted on issue #3565.
