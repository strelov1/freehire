## MODIFIED Requirements

### Requirement: Collection landing pages are indexable

The frontend SHALL serve a server-rendered landing page at `GET /collections/:slug`
for each curated collection, so the collection's job feed and its curated copy are
in the initial HTML. The page SHALL emit collection-specific document metadata: a
`<title>` and `<meta name="description">` distinct from the generic `/jobs` list,
a `<link rel="canonical">` pointing at its own `/collections/:slug` URL (never the
bare `/jobs`), and a single visible `<h1>` naming the collection. The `<title>`,
`<h1>`, and Open Graph title SHALL include the collection's live, exact open-job
count (e.g. `"1,234 React Jobs · freehire"`), sourced from the same job-list
response already fetched to render the feed — no separate count request. The page
SHALL render the collection's open jobs by pinning the collection's facet params
(e.g. `collections=<slug>`, or `work_mode`/`regions`/`countries` for an attribute
collection) as a fixed scope the visitor can further filter but cannot remove. An
unrecognised slug SHALL return a 404 (not a 200 empty or unfiltered page). Each
collection landing page SHALL be enumerated in the sitemap.

#### Scenario: A collection has its own canonical landing page

- **WHEN** `GET /collections/:slug` is requested for a known collection
- **THEN** the HTML `<head>` contains a collection-specific `<title>` that
  includes the collection's live open-job count, a `<meta name="description">`,
  and a `<link rel="canonical">` whose URL is that same `/collections/:slug`
  (not `/jobs`)

#### Scenario: The landing page shows the collection's jobs in the initial HTML

- **WHEN** `GET /collections/:slug` is requested for a known collection with open jobs
- **THEN** the returned HTML body contains a single `<h1>` naming the collection
  and its live open-job count, and the first page of that collection's job rows
  (not an empty shell)

#### Scenario: The title count matches the rendered total exactly

- **WHEN** `GET /collections/:slug` is requested for a known collection whose
  job-list response reports `meta.total` open jobs
- **THEN** the `<title>`, `<h1>`, and Open Graph title all display that same
  exact number, not a rounded or cached figure

#### Scenario: An unknown collection slug is a 404

- **WHEN** `GET /collections/:slug` is requested for a slug that maps to no collection
- **THEN** the server responds with a 404 status and an error page

#### Scenario: Collection landing pages are in the sitemap

- **WHEN** the sitemap's static-pages sub-sitemap is requested
- **THEN** it lists a `<loc>` for the `/collections` hub and for each collection's
  `/collections/:slug` landing page
