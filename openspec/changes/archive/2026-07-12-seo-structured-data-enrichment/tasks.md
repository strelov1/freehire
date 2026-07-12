## 1. Enrich company Organization JSON-LD

- [x] 1.1 RED: add `seo.test.ts` cases for `organizationJsonLd` — an enriched
  company (logo, description, website+linkedin, year_founded, employee_count,
  hq_country) emits `logo`/`description`/`sameAs`/`foundingDate`/
  `numberOfEmployees`/`address.addressCountry`; a bare company emits only
  `name`+`url` with no empty fields. Watch them fail.
- [x] 1.2 GREEN: extend `organizationJsonLd` in `web/src/lib/seo.ts` with the
  conditional fields until tests pass.

## 2. Collection landing CollectionPage + ItemList

- [x] 2.1 RED: add `seo.test.ts` cases for a new `collectionPageJsonLd` — a
  populated job list yields `CollectionPage.mainEntity` = `ItemList` with
  correct `position`/`name`/`url` per item; an empty list yields an empty
  `itemListElement`. Watch them fail.
- [x] 2.2 GREEN: implement `collectionPageJsonLd(title, description, url, jobs,
  origin)` in `seo.ts` until tests pass.
- [x] 2.3 Wire it into `web/src/routes/collections/[slug]/+page.svelte`'s
  existing `jsonLdScript([...])` block using `data.initial.jobs`; confirm with
  `svelte-check`.

## 3. Verify

- [x] 3.1 Run `vitest run`, `svelte-check`, and lint locally; all green.
- [x] 3.2 `simplify` pass over the diff; re-run tests green.
