## 1. Sitemap-based list discovery

- [ ] 1.1 Add a sitemap index parser: fetch `https://echojobs.io/sitemap.xml`, extract the `sitemap-jobs/*.xml` shard URLs in document order.
- [ ] 1.2 Add a shard parser: fetch one `sitemap-jobs/N.xml`, extract `(slug, lastmod)` pairs from each `<url><loc>/job/<slug></loc><lastmod>...</lastmod></url>` entry.
- [ ] 1.3 Replace `crawl()`'s pagination (`echojobsPageURL`, `/api/jobs?page=N`) with: walk shards in order, stop once a shard's postings age past `echojobsFreshnessWindow` — mirrors the existing stale-cutoff logic, new data source.
- [ ] 1.4 Drop `echojobsPosting`'s list-only fields that no longer come from a list call (`Title`, `CompanyName`, `URL`, `Locations`, `RemoteType`, `RequiredSkills`, `PostedAt`) — the sitemap only carries slug + lastmod now; keep just what discovery needs (slug, lastmod).

## 2. JSON-LD detail hydration

- [ ] 2.1 Add a JSON-LD extractor: given a `/job/<slug>` page's HTML body, find every `<script type="application/ld+json">...</script>` block and return the one whose decoded `@type` is `"JobPosting"`.
- [ ] 2.2 Define the `JobPosting` JSON-LD struct: `title`, `hiringOrganization.name`, `description`, `jobLocationType`, `jobLocation.address.{addressLocality,addressCountry}`, `applicantLocationRequirements.name`, `skills` (string), `datePosted`.
- [ ] 2.3 Replace `detail()`'s request (`echojobsDetailURL`, JSON decode) with `GET /job/<slug>` + the JSON-LD extractor from 2.1/2.2.
- [ ] 2.4 Map JSON-LD fields to `Job`: `Remote`/`WorkMode` from `jobLocationType == "TELECOMMUTE"` (else `WorkMode: ""`); `Location` from `jobLocation.address` or `applicantLocationRequirements.name` fallback; `Skills` from splitting the comma-separated `skills` string through `skilltag.Canonicalize`; `PostedAt` from `datePosted` via `time.Parse(time.RFC3339, ...)`; `Description` sanitized as before (`sanitizeHTML`).
- [ ] 2.5 Update `FetchNew`'s per-posting flow: unseen postings get the full JSON-LD hydration (2.3/2.4) building both list-derived and detail-derived fields from the SAME JSON-LD response (title/company/etc. no longer come from a separate list call — see task 1.4); seen postings stay list-only (`SeenRefresh: true`, `ExternalID` only, `Title` left empty — ref design.md's "Refresh strategy" decision).

## 3. EchoJobsDescription (backfill) and liveness

- [ ] 3.1 Update `EchoJobsDescription` to fetch `/job/<slug>` and extract the JSON-LD description (task 2.1/2.2's extractor), same sanitize-and-return-`ok` contract as before.
- [ ] 3.2 Update `cmd/liveness/echojobs.go`'s `checkEchoJobsLiveAt`: replace the `/api/jobs/%s` + `{"error":"job fetch failed"}` body check with `GET /job/<slug>` + HTTP status (200 → `liveness.Live`, 404 → `liveness.Expired`, anything else → `liveness.Uncertain`).

## 4. Tests

- [ ] 4.1 Replace `echojobs_test.go`'s JSON list/detail fixtures with an httptest server serving sitemap-index XML, shard XML, and `/job/<slug>` HTML-with-embedded-JSON-LD fixtures.
- [ ] 4.2 Cover: freshness-window cutoff across shard boundaries; unseen-posting hydration (full JSON-LD → Job mapping, including the `TELECOMMUTE` remote signal and comma-separated skills split); seen-posting refresh (empty Title, `SeenRefresh: true`, no detail fetch made — assert the test server's detail endpoint was never hit); a JSON-LD block missing/malformed on a detail page (falls back to list-only ingestion, same as today's failed-detail behavior).
- [ ] 4.3 Update `cmd/liveness`'s echojobs test for the new URL + status-code liveness signal.

## 5. Manual verification (post-deploy)

- [ ] 5.1 Deploy, confirm the next `freehire-ingest@echojobs.timer` fire ingests postings with non-empty descriptions.
- [ ] 5.2 Run `go run ./cmd/backfill-echojobs` once by hand to repair rows accumulated empty during the outage (per design.md's Migration Plan); follow with `make reindex`.
