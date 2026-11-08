## Context

`internal/sources` holds one adapter per ATS behind the `Source` interface; several
(smartrecruiters, rippling, bamboohr, gem) use the shared `fetchDetails` bounded pool when the
list lacks the description. Join.com fits this list-API → detail-API shape exactly, over a
public JSON surface — the first fully-JSON adapter added since gem. No new HTTP transport is
needed (`GetJSON` + `fetchDetails` already exist); one new library renders the Markdown body.

Contract confirmed live against company `167291` (Just Hiring, slug `justhiringde`):

- `GET /api/public/companies/<companyId>/jobs?page=1&pageSize=100` → `{items, aggregations,
  pagination{rowCount, pageCount, page, pageSize}}`. Each item: `id` (numeric), `idParam`
  (`<id>-<slug>`), `title`, `createdAt` (RFC3339), `city {cityName, countryName}`,
  `workplaceType` ("REMOTE"/…), `remoteType`. The **slug path returns empty** — the API keys by
  numeric company id. `pagination.pageCount` is authoritative (the page's `__NEXT_DATA__`
  carried a misleading larger `total`; the API count is trusted instead).
- `GET /api/public/jobs/<id>` → the full job: `description` (**Markdown**, e.g. `*   ` bullets,
  `\n\n` paragraphs), `idParam`, and `company {domain, name}` (`domain` is the slug). The HTML
  career page's JSON-LD carries an HTML description, but the static listing anchors there are
  **stale** (an old id 301-redirects to the canonical one) — the JSON API returns canonical ids,
  so it is the reliable source.

## Goals / Non-Goals

**Goals:**

- A `join` adapter that enumerates a company's jobs from the public list API (full pagination)
  and yields normalized jobs with Markdown-rendered, sanitized-HTML descriptions, reusing the
  list→detail pattern and `fetchDetails`.

**Non-Goals:**

- HTML / `__NEXT_DATA__` scraping (rejected — the public JSON API is cleaner and stable).
- Structured department/category mapping (left to enrichment).
- The authenticated `/api/companies/<id>/jobs` endpoint (401; the public one suffices).

## Decisions

- **Board = numeric Join company id.** The API keys jobs by company id (the slug path returns
  empty), so `e.Board` is that id (e.g. `167291`); the config comment carries the slug/name.
  The public job URL uses the slug, read per job from the detail response's `company.domain`.
- **Enumerate via the list API, paginating by pageCount.** `Fetch` GETs
  `…/companies/<board>/jobs?page=N&pageSize=100` for N = 1…`pagination.pageCount` (GetJSON),
  collecting items. `pageSize=100` keeps page counts low; the loop trusts the API's pageCount.
- **Detail via the per-job API.** `fetchDetails(items, workers, …)` GETs `…/jobs/<item.id>`
  (GetJSON) for the `description` and `company.domain`; a failed request drops only that posting.
- **Markdown → HTML → sanitize.** The job body is Markdown; the adapter renders it with
  **goldmark** (`goldmark.Convert`) and then `sanitizeHTML`s the result — the idiomatic library
  over a fragile newline/regex shim, consistent with "descriptions are sanitized HTML".
- **Job mapping:**
  - `ExternalID` = the API job `id` (canonical; pipeline namespaces it by board).
  - `URL` = `https://join.com/companies/<company.domain>/<idParam>`.
  - `Title` = item `title`; `Company` = `e.Company`.
  - `Location` = `joinNonEmpty(city.cityName, city.countryName)` from the list item (may be "").
  - `Description` = `sanitizeHTML(markdownToHTML(detail.description))`.
  - `Remote` = `workplaceType == "REMOTE"` OR `isRemote(joinNonEmpty(location, title))`.
  - `PostedAt` = `parseRFC3339(item.createdAt)`.

## Risks / Trade-offs

- **Undocumented public API.** `/api/public/...` is not a contracted integration, so a shape
  change could break the adapter. Mitigation: it is the same surface the site itself consumes
  (more stable than scraping markup or the internal `__NEXT_DATA__` state); a decode/HTTP error
  drops the affected posting (detail) or fails the board cleanly (list), never silently.
- **Markdown rendering fidelity.** goldmark renders CommonMark; Join's bodies are standard
  Markdown (bullets, paragraphs, bold). Edge constructs degrade to text, then sanitize. Covered
  by a table-driven test asserting lists/paragraphs survive and active content is stripped.
- **Opaque board id.** A numeric company id is less readable than a slug; mitigated by the
  config comment (slug + name) and a one-time lookup when adding a board.
- **Per-job detail fetches** — one GET per job under the bounded pool, like the other
  detail-fetching adapters; acceptable for a scheduled crawl.
- **Empty location** for remote/unspecified jobs — accepted; enrichment derives location from
  the description.
