## 1. HTTP plumbing

- [x] 1.1 Add `PostFormWithHeaders(ctx, url, headers map[string]string, values url.Values) (*html.Node, error)` to `internal/sources/http.go`, form-encoding `values` as the request body (`application/x-www-form-urlencoded`) and parsing the response as HTML (mirrors `PostJSONWithHeaders`'s shape but for a form body + HTML response, per design.md Decision 4).

## 2. Adapter skeleton and registration

- [x] 2.1 Create `internal/sources/aijobs.go`: `aijobs` struct, `NewAijobs` constructor over a cookie-jar-backed client, `Provider() string { return "aijobs" }`, `boardless()`, `aggregator()` markers (per spec Requirement "aijobs.net is crawled as a boardless aggregator").
- [x] 2.2 Register `"aijobs"` in the source registry (`internal/sources/registry.go`), built with `newCookieClient()`.
- [x] 2.3 Create `sources/aijobs.yml` with one placeholder boardless entry, mirroring `sources/arbeitnow.yml`'s convention.

## 3. CSRF session bootstrap and listing pagination

- [x] 3.1 Implement session bootstrap: a `GET` to `https://aijobs.net/` to acquire the `csrftoken` cookie once per run (per spec Requirement "The listing session is authenticated per run via a CSRF cookie").
- [x] 3.2 Implement the paginated listing walk: `POST /?page=N` via `PostFormWithHeaders`, echoing the csrftoken as the `x-csrftoken` header and `csrfmiddlewaretoken` form field, with the required `Referer` header; extract each page's job links (slug + numeric external id) from the response HTML.
- [x] 3.3 Implement the pagination stop conditions: stop when a page's postings are ALL already reported by `seen`, or when a hard `aijobsMaxPages` cap is reached (per spec Requirement "Listing pagination is bounded by a seen-page stop and a hard page cap").

## 4. Detail-page parsing

- [x] 4.1 Parse the company display name from the detail page's `/company/<slug>-<id>/` link: strip the trailing `-<id>`, title-case the hyphen-split slug; drop the posting if no company link is present (per spec Requirement "Company display name is derived from the company profile URL slug").
- [x] 4.2 Parse `Job.Description` from the "Tasks" section's list items as a bullet list, and `Job.Skills` from the "Skills/Tech-stack" section's anchor texts; omit a "Perks/Benefits" section whose only item is `N/A`; do not parse or store the salary badge (per spec Requirement "Description and skills are built from the page's structured sections, salary is dropped").
- [x] 4.3 Parse `Job.Remote` from the location badge marked `R`, and `Job.Title`/`Job.Location` from the page header.

## 5. Relative-time parsing (PostedAt)

- [x] 5.1 Implement parsing of the "Found X<unit> ago" text into `Job.PostedAt`, handling at minimum `h` (hours) and `d` (days); an unrecognized unit leaves `PostedAt` nil rather than failing the posting (per spec Requirement "Posted time is parsed from the relative-time string"). **Note:** the month/year unit-conversion approximation (calendar-aware `time.AddDate` vs. a flat-day duration) is a deliberate design choice left for the user to make when writing this task.

## 6. Hydrating fetch and per-run budget

- [x] 6.1 Implement `Fetch` for use when no `seen` predicate is supplied. Shipped as a hydrating fallback, not the originally-planned list-only one: aijobs's listing carries no company, and a company-less posting is dropped, so a genuinely detail-less `Fetch` could never return a usable `Job`. It delegates to `FetchNew` with a predicate reporting everything unseen instead — see design.md Decision 1 and the spec's "Detail fetches happen only for postings not already in the catalogue" Requirement, both updated to match.
- [x] 6.2 Implement `FetchNew(ctx, e, seen)`: walk the listing (§3), skip a detail fetch for any external id `seen` reports true, and fetch detail (§4, §5) for the rest via the shared `fetchDetails` bounded-worker helper.
- [x] 6.3 Add the `AIJOBS_MAX_NEW_PER_RUN` env var (default 500): once that many unseen postings are queued for detail fetch in one run, stop issuing further detail requests and stop discovering further listing pages (per spec Requirement "New-posting detail fetches are bounded per run").

## 7. Verification

- [x] 7.1 Run `go vet ./...` and `go vet -tags=integration ./...`; run `go test ./internal/sources/...`.
- [x] 7.2 Run `go run ./cmd/ingest sources/aijobs.yml` against a local `DATABASE_URL` and confirm a bounded first crawl ingests jobs with company/description/skills populated and no salary field set.
