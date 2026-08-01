## 1. Split the two registries in `internal/sources`

- [x] 1.1 RED: in `usajobs_test.go`, `reed_test.go` and `whatjobs_test.go`, restate each
      `…RegisteredOnlyWhenKeySet` test as the crawl/taxonomy split — with the variable unset,
      `All(nil)` MUST contain the provider and `All(<a stub client>)` MUST NOT; with it set,
      both do. Assert `FilterableProviders()` lists the provider with the variable unset.
- [x] 1.2 RED: add a test that the taxonomy registry is total — `AggregatorProviders(All(nil))`
      contains `whatjobs`, `reed` and `usajobs`, and `ProviderKind(All(nil), …)` answers
      `KindAggregator` for each — with all three variables explicitly unset.
- [x] 1.3 GREEN: in `registry.go`, register the three on the `c == nil` path with an empty
      credential, mirroring the `taleo` / `meta` branches below them; keep the credential gate
      on the client path. Rewrite the two comment blocks to name the crawl registry and the
      taxonomy registry rather than "registered / absent".
- [x] 1.4 Add `sources.Taxonomy()` — a wrapper over `All(nil)` carrying the "no transport, never
      Fetch here; crawlers call All" rule — and point `FilterableProviders` at it.
- [x] 1.5 Confirm `filterableProviders` and `SweepGraceWindows` still behave: `whatjobs` keeps
      its 14-day sweep grace on the crawl path, and `reed`/`usajobs` (boardless aggregators)
      stay in the facet.

## 2. Move the classifying call sites onto `Taxonomy()`

- [x] 2.1 `cmd/reindex/main.go` — call `sources.Taxonomy()`, and delete the stale rationale
      comment at :420-424 (it asserts `usajobs` is the only keyed adapter and that its absence
      is harmless) in favour of one line saying where the aggregator set comes from.
- [x] 2.2 `cmd/ghost-crosscheck/main.go:83-95` — call `sources.Taxonomy()`; check the empty-list
      guard and the comment above it still read true.
- [x] 2.3 `internal/handler/status.go:123` — call `sources.Taxonomy()`, dropping the "nil client
      is safe" half of the comment that the name now carries. Confirm `classify_test.go` covers
      the three providers now that they are always present; extend it if not.
- [x] 2.4 Grep for any remaining `sources.All(nil)` outside the sources package and convert it,
      so the two entry points cannot drift back together.

## 3. Frontend contract and labels

- [x] 3.1 Run `make gen-contracts` with the three variables unset and verify `SOURCE_VALUES`
      gains exactly `reed`, `usajobs` and `whatjobs` and nothing else changes in the file.
- [x] 3.2 Adjacent fix, not caused by S7: add `usajobs: 'USAJobs'` and `whatjobs: 'WhatJobs'` to
      `SOURCE_LABELS` in `web/src/lib/facets.ts`. The source facet is distribution-driven, so it
      already offers all three and renders the first two through the title-case fallback as
      `Usajobs` / `Whatjobs`; `reed` → `Reed` is already correct.
- [x] 3.3 Point `StatusBoard.svelte` at `sourceLabel` instead of its own local `titleCase` —
      the /status page is a fifth surface rendering provider codes under its own spelling, and
      the shared fallback also title-cases every word (`habr_career` → "Habr Career").
- [x] 3.4 Run `pnpm run check` in `web/` and confirm it is no worse than the baseline (the web
      lint baseline is red — compare, do not expect zero).

## 4. Documentation

- [x] 4.1 Rewrite `internal/sources/AGENTS.md:12` around the crawl/taxonomy split — the
      credential gates the crawl registry, the taxonomy registry is total — and say why
      (`cmd/reindex`'s aggregator set, the status page's kind, the generated source facet).
- [x] 4.2 Replace the stale one-adapter claim at `internal/pipeline/AGENTS.md:11` with a
      pointer to the sources rule rather than a second copy of it.

## 5. Verify and close

- [x] 5.1 `go build ./... && go vet ./... && go test ./...` green, including with all three
      variables unset (the default local environment).
- [x] 5.2 Mark S7 ✅ in `docs/reviews/2026-08-01-architecture-review.md` — the shortlist row,
      the `S7` heading and the Progress table — with a note of anything the finding understated.
