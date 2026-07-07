## ADDED Requirements

### Requirement: Collection landing pages are indexable

The frontend SHALL serve a server-rendered landing page at `GET /collections/:slug`
for each curated collection, so the collection's job feed and its curated copy are
in the initial HTML. The page SHALL emit collection-specific document metadata: a
`<title>` and `<meta name="description">` distinct from the generic `/jobs` list,
a `<link rel="canonical">` pointing at its own `/collections/:slug` URL (never the
bare `/jobs`), and a single visible `<h1>` naming the collection. The page SHALL
render the collection's open jobs by pinning the collection's facet params (e.g.
`collections=<slug>`, or `work_mode`/`regions`/`countries` for an attribute
collection) as a fixed scope the visitor can further filter but cannot remove. An
unrecognised slug SHALL return a 404 (not a 200 empty or unfiltered page). Each
collection landing page SHALL be enumerated in the sitemap.

#### Scenario: A collection has its own canonical landing page

- **WHEN** `GET /collections/:slug` is requested for a known collection
- **THEN** the HTML `<head>` contains a collection-specific `<title>`, a
  `<meta name="description">`, and a `<link rel="canonical">` whose URL is that
  same `/collections/:slug` (not `/jobs`)

#### Scenario: The landing page shows the collection's jobs in the initial HTML

- **WHEN** `GET /collections/:slug` is requested for a known collection with open jobs
- **THEN** the returned HTML body contains a single `<h1>` naming the collection
  and the first page of that collection's job rows (not an empty shell)

#### Scenario: An unknown collection slug is a 404

- **WHEN** `GET /collections/:slug` is requested for a slug that maps to no collection
- **THEN** the server responds with a 404 status and an error page

#### Scenario: Collection landing pages are in the sitemap

- **WHEN** the sitemap's static-pages sub-sitemap is requested
- **THEN** it lists a `<loc>` for the `/collections` hub and for each collection's
  `/collections/:slug` landing page

### Requirement: Public list pages carry a primary heading

The public list pages (`/jobs` and `/companies`) SHALL each render exactly one visible `<h1>` describing the page's content, so the primary heading is present for search engines and assistive technology.

#### Scenario: The jobs list has a top-level heading

- **WHEN** `GET /jobs` is requested
- **THEN** the returned HTML body contains exactly one `<h1>` describing the jobs list

#### Scenario: The companies list has a top-level heading

- **WHEN** `GET /companies` is requested
- **THEN** the returned HTML body contains exactly one `<h1>` describing the companies list
