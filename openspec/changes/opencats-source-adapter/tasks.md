## 1. Adapter — listing

- [x] 1.1 Add `internal/sources/opencats_test.go` with builder functions for two fixture
      shapes: the stock XHTML template and a rewritten template with different classes and
      column order. Assert the listing yields one posting per `p=showJob&ID=<n>` link, with
      the anchor text as title and the captured `<n>` as `ExternalID`, identically for both
      shapes.
- [x] 1.2 Add `internal/sources/opencats.go` with `NewOpencats`, `Provider() == "opencats"`,
      and listing parsing that satisfies 1.1.
- [x] 1.3 Test and implement board resolution: a root-mounted board
      (`atscareers.g4s.com`) and a path-prefixed board (`careers.boomit.pt/careers`) both
      build the correct listing URL over HTTPS.
- [x] 1.4 Test and implement de-duplication of repeated posting links within one listing
      (title link plus separate apply link → one posting).
- [x] 1.5 Test and implement exclusion of the general-application entry ("Can't find what
      you're looking for? Apply here").
- [x] 1.6 Test and implement listing-fetch failure: an unreachable listing returns an error
      for the board.

## 2. Adapter — detail

- [x] 2.1 Test and implement detail-page parsing: location and description read from the
      detail page, description passed through `sanitizeHTML` (assert an embedded `<script>`
      is stripped).
- [x] 2.2 Test and implement per-posting failure isolation: one failing detail page skips
      that posting and leaves the rest of the board intact.
- [x] 2.3 Wire the detail fan-out through the shared `fetchDetails` helper at
      `defaultDetailWorkers`.
- [x] 2.4 Register the adapter in `sources.All` under key `opencats`; confirm it is absent
      from `sources.SelfClosingProviders`.

## 3. Harvest prober

- [ ] 3.1 Add `cmd/harvest-boards/opencats_prober_test.go`: probing a host whose portal lists
      postings returns the company name and a positive count; a host with an unreachable or
      empty portal returns a silent skip (`"", 0, nil`).
- [ ] 3.2 Add `cmd/harvest-boards/opencats_prober.go` implementing `prober` to satisfy 3.1,
      including company name from the portal page title with a host fallback.
- [ ] 3.3 Test and implement candidate filtering: `*.catsone.com` hosts, bare IP addresses,
      and the project's own documentation/demo sites are rejected.
- [ ] 3.4 Test and implement `discoverer`: results from the page-title and URL-routing
      signature queries are unioned and de-duplicated into one candidate host list.
- [ ] 3.5 Register the prober in the `probers` map so `go run ./cmd/harvest-boards opencats`
      runs without a seed file.

## 4. Harvest run and board file

- [ ] 4.1 Run `go run ./cmd/harvest-boards opencats` and review the resulting diff.
- [ ] 4.2 Correct the proposed company names by hand where the portal title is unhelpful
      (e.g. a title that is just the hostname), and add the provider header comment to
      `sources/opencats.yml` describing the board format.
- [ ] 4.3 Verify the board file loads: `sources.LoadConfig` validation passes for every entry.

## 5. Verification

- [ ] 5.1 `go build ./... && go vet ./... && go test ./...` all green.
- [ ] 5.2 Crawl one real board end-to-end against the live portal and confirm titles,
      locations, and descriptions are populated and sane.
- [ ] 5.3 Confirm no cross-provider duplication: no board in `sources/opencats.yml` also
      appears in `sources/catsone.yml`.
