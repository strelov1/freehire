## 1. `platform/firecrawl` — the client

- [x] 1.1 Write the failing tests against an `httptest` stand-in for the vendor: a page comes
      back with its bytes; a TARGET refusal is reported as the target's status, not the
      vendor's 200; a VENDOR failure is a distinct error; a missing key is refused at
      construction.
- [x] 1.2 Write the failing test for the budget: with a budget of N, the (N+1)th fetch fails
      with a named error and **issues no request** (the stand-in counts).
- [x] 1.3 Implement `Client` + `Fetch(ctx, url) (status int, body []byte, err error)` with the
      budget counted in the client, documenting why it lives there and not per adapter.
- [x] 1.4 Register `firecrawl` in the `platform` block, with the comment saying it is the
      `browseruse` category (vendor API transport) and NOT `browser` (our own process, free).

## 2. `sources` — the hosted-fetch getter

- [x] 2.1 Write the failing test: the getter satisfies `XMLGetter` + `HTMLGetter`, decodes XML,
      parses HTML, and turns a non-2xx target status into `*StatusError` carrying that code.
- [x] 2.2 Implement it, reusing `browserStatusError`'s shape so the existing helpers keep
      reading it.

## 3. The tier, and the deliberate override

- [x] 3.1 Write the failing tests: no key is a no-op; a key rewires `bayt` and `gulftalent`;
      `gulftalent` resolves to hosted WITH a key and to the fingerprint client WITHOUT one; an
      unrelated registry is untouched; an unparseable budget fails the run.
- [x] 3.2 Narrow the browser tier's disjointness test to browser-vs-proxy, and add the hosted
      tier's own rule: it MAY override, and the test states which providers it deliberately
      takes over and why.
- [x] 3.3 Add `firecrawlProviders` + `ApplyFirecrawlEgress`; wire it into `cmd/ingest` LAST,
      after the proxy and browser tiers, with the comment saying the order is load-bearing.
- [x] 3.4 Replace `proxy.go`'s "waiting for a genuinely residential pool" note for bayt and
      gulftalent with what actually answered it.

## 4. Ship

- [x] 0 `gofmt -w`, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`.
      `bayt` and `gulftalent` tests must pass **unedited**.
- [x] 0 Document the tier in `internal/ingest/sources/AGENTS.md` and the client in its own
      AGENTS.md; add the module-file row; record the measured 1% yield so the next person
      inherits the number and not just the switch.
- [ ] 4.3 Open the PR on top of the browser-tier branch, stating that merging spends nothing
      and that three separate acts are needed before it does.
