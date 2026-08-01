## 1. Registry: the backer kind

- [ ] 1.1 Add `KindBacker` to `internal/collections`, and move `yc` and `techstars`
      onto it. Registry test asserts every entry's kind is one of the three, and
      that `yc`/`techstars` are backers.
- [ ] 1.2 Extend `Dataset` with the self-fetching `Records` form and teach `Valid()`
      to reject it alongside any other payload form. Test covers each single form
      accepted and each two-form combination rejected.

## 2. The Speedrun directory as a membership source

- [ ] 2.1 Parse one directory page into records (name plus tier), from a fixture of
      the real payload. Test asserts an entry missing a name or tier is dropped
      rather than yielding a nameless record.
- [ ] 2.2 Walk every page: read `total_pages` and fetch each, failing the whole
      fetch when any page fails. Test asserts a mid-walk failure is an error, not a
      short membership.
- [ ] 2.3 Filter by tier, one resolver per tag. Test named for what it prevents:
      a `market`-tier company (fixture uses a real one, e.g. Walmart) earns neither
      a16z tag.
- [ ] 2.4 Register `a16z-portfolio` and `a16z-speedrun` with their titles,
      descriptions and `Records` sources. Registry test asserts both resolve to
      the expected tier.

## 3. Import worker

- [ ] 3.1 Resolve the `Records` form in `cmd/import-collections`. Test asserts a
      self-fetching dataset's records reach the plan like any other dataset's.
- [ ] 3.2 Run `go build ./... && go vet ./... && go test ./...`, then
      `cmd/import-collections -dry-run` against prod data. Read the report for both
      new tags; spot-check matched names for slug collisions on short names
      (`Dex`, `Sekai`, `Emanate`). Record the counts in the change before writing.

## 4. Frontend registry and badge

- [ ] 4.1 Regenerate contracts (`cmd/gen-contracts`) and confirm `kind: 'backer'`
      reaches `web/src/lib/generated/contracts.ts` with no generator change.
- [ ] 4.2 Commit the three brand SVGs under `web/src/lib/brands/` as Svelte
      components taking a `class`.
- [ ] 4.3 `web/src/lib/backers.ts` — resolve a collection list to badges in registry
      order. Vitest covers: a backer tag yields its badge; an editorial or
      credential tag yields nothing; an unknown backer slug with no committed mark
      yields nothing (no placeholder).
- [ ] 4.4 `BackerBadge.svelte` — render the mark with an accessible name naming the
      brand, no monogram fallback.

## 5. Surfaces

- [ ] 5.1 Job feed card: badge beside the company name in `JobRow.svelte`, signal
      row untouched.
- [ ] 5.2 Job page: badge with text beside the company, linking to the collection
      landing.
- [ ] 5.3 Company page (`CompanyHeader.svelte`) and `/companies` cards.
- [ ] 5.4 `/collections` hub cards and the collection landing header.
- [ ] 5.5 Filter chips: optional `icon` on `FacetOption`, rendered by
      `PillGroup.svelte`; wire the marks for the four backer collections. Vitest
      covers a facet option without an icon rendering unchanged.

## 6. Verify and ship

- [ ] 6.1 Run the import for real, then `make reindex`. Confirm the `collections`
      facet distribution shows both new tags.
- [ ] 6.2 Visual check of the feed, job page, company page and filter modal at
      <500px and at desktop width (headless Chrome; `--window-size` is unreliable
      below 500px, so verify the narrow case deliberately).
- [ ] 6.3 Offer a `/blog` changelog entry for the shipped feature.
