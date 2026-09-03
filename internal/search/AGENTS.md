# internal/search

The Meilisearch index topology, the incremental drain into it, saved searches, the facet snapshot, and intent parsing.

**Layer 6 of 8.**

May import: `platform`, `dict`, `ai`, `identity`, `candidate`, `job` — and itself.

Must NOT import: `application`, `engage`, `ingest`, `api`.

`application` share this layer, and the ban runs both ways: two blocks that can see each other are one block under two names.

Both directions are enforced. `depguard` in `.golangci.yml` fails on the
offending import line; `internal/platform/arch/layering` holds the same table and
reports the whole graph at once, including imports that exist only in test files.

## Packages

`facetsnapshot` `savedsearch` `search` `searchdrain` `searchintent` `similarjobs`
`suggest`

## The suggestions dictionary (`suggest`)

The search box's completions, in a SEPARATE Meilisearch index (`suggestions`) built
offline by `cmd/build-suggestions`.

It exists because the facet dictionaries cannot answer what people type. 8,680
postings are titled "Product Owner" and the role vocabulary has no such role — only an
alias folding it into `product`, whose label is "Product Manager" — so the box answered
a question about real postings by renaming the person asking. Titles are the vocabulary
the market actually uses.

**Not a facet on `jobs`.** `title` is searchable there but not filterable, distinct
titles number in the millions, and `MaxValuesPerFacet` truncates a facet's
distribution. Its smallness is also the performance argument: the box queries it per
settled keystroke, and doing that against 8M documents would put a per-keystroke query
on the index serving the whole site.

Three things are worth knowing before changing it:

- **`Title` is ONE function**, applied to mined titles, to typed queries recorded in
  `search_queries`, and to the phrase recognition. A second copy drifts, and the drift
  looks exactly like a suggestion nobody ever searches for.
- **The floor runs AFTER normalisation.** "Product Owner", "product owner" and "PRODUCT
  OWNER" are three catalogue rows and one suggestion; a floor applied before merging
  drops a title that clears it comfortably after.
- **Recognition is exact, completion is fuzzy**, and the split is deliberate. A
  mistyped phrase must fall into the fragment where Meilisearch forgives it; consuming
  it as recognised means the typo is never corrected. Reimplementing the fuzzy half
  here would be the second matcher this package was built to retire.

An empty query returns nothing, and that is a boundary. What an empty box offers is the
filter modal's curated category order, which lives in `web/src/lib/filterSections.ts`
and is checked there against the category vocabulary at compile time.
