## 1. Adapter mapping (single page)

- [x] 1.1 Add `internal/sources/djinni_test.go` with a compact djinni-shaped listing fixture
  (one `ld+json` JobPosting array + noise blocks) and a RED test asserting the adapter maps its
  JSON-LD `JobPosting` array to `Job`s with the right fields (title, sanitized description,
  company from `hiringOrganization.name`, country from
  `applicantLocationRequirements.address.addressCountry`, `identifier` → `ExternalID`,
  `url` → `URL`, `datePosted` → `PostedAt`).
- [x] 1.2 Create `internal/sources/djinni.go`: a `djinni` struct over the HTML client, a
  `djinniPosting` struct for the schema.org fields, and a `toJob` mapper that reuses the shared
  `LDJobPostings` extractor and `sanitizeHTML`. Make 1.1 green.
- [x] 1.3 Add the drop-rule test + code: a `JobPosting` with no `identifier`, no `url`, or no
  company is dropped (not yielded), and one bad posting does not abort the page.

## 2. Pagination & crawl

- [x] 2.1 RED test: `Fetch` pages from 1 upward and stops at the end of the feed. Cover BOTH
  stops: the past-the-end 302→/jobs/ redirect (final URL loses the `page=N` marker; must NOT
  re-ingest the redirected page-1 content) and a non-redirected empty page.
- [x] 2.2 Implement the page loop in `djinni.go` using `GetHTMLResolved` with the redirect-based
  end-of-feed detection and a `djinniMaxPages` backstop constant; make 2.1 green. (Added the
  `HTMLResolvedGetter` interface to the `HTTPClient` composition.)

## 3. Classification & registration

- [x] 3.1 Add the `boardless()` and `aggregator()` markers to `djinni`; test that
  `ProviderKind(All(nil), "djinni") == KindAggregator` and that `"djinni"` appears in
  `AggregatorProviders(All(nil))` (so it inherits the reindex ATS-suppression pass).
- [x] 3.2 Register `djinni` in `sources.All` (one line) and add `Provider()` returning
  `"djinni"`. Verify `go build ./... && go vet ./...`.

## 4. Board file & docs

- [x] 4.1 Add `sources/djinni.yml` with a single boardless entry (`company: Djinni`,
  `provider: djinni`) and confirm it validates against the registry (LoadConfig + Validate).
- [x] 4.2 No AGENTS.md change: `internal/sources/AGENTS.md` lists adapters only illustratively
  (`greenhouse.go, lever.go, ashby.go, …`), not by class — nothing to enumerate.

## 5. Verify end-to-end

- [x] 5.1 Ran `go test ./internal/sources/...` (green) and live smoke checks against djinni.co:
  a real captured listing page maps 15 jobs, and the live Go client's `GetHTMLResolved` confirms
  the redirect contract (page 2 keeps its marker; page 489 redirects to /jobs/, marker gone).
