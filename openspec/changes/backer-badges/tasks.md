## 1. Registry: the backer kind

- [x] 1.1 Add `KindBacker` to `internal/collections`, and move `yc` and `techstars`
      onto it. Registry test asserts every entry's kind is one of the three, and
      that `yc`/`techstars` are backers.
- [x] 1.2 Extend `Dataset` with the self-fetching `Records` form and teach `Valid()`
      to reject it alongside any other payload form. Test covers each single form
      accepted and each two-form combination rejected.

## 2. The Speedrun directory as a membership source

- [x] 2.1 Parse one directory page into records (name plus tier), from a fixture of
      the real payload. Test asserts an entry missing a name or tier is dropped
      rather than yielding a nameless record.
- [x] 2.2 Walk every page: read `total_pages` and fetch each, failing the whole
      fetch when any page fails. Test asserts a mid-walk failure is an error, not a
      short membership.
- [x] 2.3 Filter by tier, one resolver per tag. Test named for what it prevents:
      a `market`-tier company (fixture uses a real one, e.g. Walmart) earns neither
      a16z tag.
- [x] 2.4 Register `a16z-portfolio` and `a16z-speedrun` with their titles,
      descriptions and `Records` sources. Registry test asserts both resolve to
      the expected tier.

## 3. Import worker

- [x] 3.1 Resolve the `Records` form in `cmd/import-collections`. Test asserts a
      self-fetching dataset's records reach the plan like any other dataset's.
- [x] 3.2 Run `go build ./... && go vet ./... && go test ./...` — all green.
      (The prod dry-run moves to 6.1, where it belongs with the real import: it
      needs the live catalogue, and the frontend does not depend on it.)

## 4. Frontend registry and badge

- [x] 4.1 Regenerate contracts (`cmd/gen-contracts`) and confirm `kind: 'backer'`
      reaches `web/src/lib/generated/contracts.ts` with no generator change.
- [x] 4.1a Restore the collection filter options: `facets.ts:418` builds them from
      `kind === 'editorial'` plus `kind === 'credential'`, so regenerating drops
      `yc` and `techstars` out of the filters entirely. Test first — a backer
      collection must appear among the `collections` facet options.
- [x] 4.2 Commit the three brand marks under `web/static/brands/` — each brand's own
      square icon from its own site (PNG, 5.5 KB total). SVG was the plan; a16z's
      mark is a circular figure that redrawing would only approximate, and the badge
      renders at 16–20px where the raster is indistinguishable.
- [x] 4.3 `web/src/lib/backers.ts` — resolve a collection list to badges in registry
      order. Vitest covers: a backer tag yields its badge; an editorial or
      credential tag yields nothing; an unknown backer slug with no committed mark
      yields nothing (no placeholder).
- [x] 4.4 `BackerBadge.svelte` — render the mark with an accessible name naming the
      brand, no monogram fallback.

## 5. Surfaces

- [x] 5.1 Job feed card: badge beside the company name in `JobRow.svelte`, signal
      row untouched.
- [x] 5.2 Job page: badge with text beside the company, linking to the collection
      landing.
- [x] 5.3 Company page (`CompanyHeader.svelte`) and `/companies` cards. The list
      endpoint did not return `collections` at all — its SQL selected six columns —
      so the column, the sqlc row, and the Meilisearch projection of the same wire
      shape all had to carry it before the card could render a mark.
- [x] 5.4 `/collections` hub cards and the collection landing header.
- [x] 5.5 Filter chips: optional `icon` on `FacetOption`, rendered by
      `PillGroup.svelte`; wire the marks for the four backer collections. Vitest
      covers a facet option without an icon rendering unchanged.

## 6. Verify and ship

- [x] 6.1 `cmd/import-collections -dry-run` against the live catalogue. Read the
      report for both new tags and spot-check matched names for slug collisions on
      short ones (`Dex`, `Sekai`, `Emanate`); add a gate from the directory's
      `location` field if they show up. Only then run the import for real, followed
      by `make reindex`, and confirm the `collections` facet shows both new tags.
- [x] 6.2 Visual check of the feed, job page, company page and filter modal at
      <500px and at desktop width (headless Chrome; `--window-size` is unreliable
      below 500px, so verify the narrow case deliberately).
- [x] 6.3 Offer a `/blog` changelog entry for the shipped feature.

## Outcome (2026-08-01)

Shipped as #1383 + #1384. Import on prod: `a16z-portfolio` 258 companies of 281,
`a16z-speedrun` 33 of 36 — 324 companies and 25,422 jobs updated. The dry-run's
collision check came back clean: of 317 directory names 213 are short or
single-word, but only `convex` matched the wrong company (convex.com, not a16z's
convex.dev) and it carries zero jobs, so the catalogue never shows it.

Two things the run itself uncovered:

- The directory drops the connection for Go's default user agent, which surfaced
  as a bare `status 500` while `curl` from the same host returned 200. Fixed in
  #1384. The abort held: both failed runs wrote nothing.
- `freehire-reindexw.service` had been **failed since 2026-07-31** — the disk
  floor (`REINDEX_MIN_FREE_GB=70`) against 68 GiB free — so prod facets had been
  frozen for four days and nothing reported it. Freed by rotating a 3.9 GiB
  `/var/log/syslog`; rebuild then started with the timer stopped per the
  collision rule, with a watcher unit to restart the timer when it finishes.
