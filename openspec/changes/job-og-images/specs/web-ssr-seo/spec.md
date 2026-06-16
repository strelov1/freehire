## MODIFIED Requirements

### Requirement: Per-route document metadata

Each server-rendered page SHALL emit route-specific document `<head>` metadata in
its initial HTML: a descriptive `<title>`, a `<meta name="description">`, a
canonical URL, and Open Graph / Twitter Card tags. The job-detail page's title
and description SHALL derive from the job (e.g. title and company), replacing the
single static `freehire` title used for every URL.

The job-detail page SHALL additionally emit an `og:image` pointing at that job's
preview image (`/jobs/:slug/og.png`) as an absolute URL, with its declared
dimensions (1200×630) and an `og:image:alt`, and SHALL use the
`summary_large_image` Twitter Card with a matching `twitter:image`. The metadata
component SHALL treat the preview image as optional: routes that pass no image
keep the prior `summary` Twitter Card behaviour and emit no `og:image`.

#### Scenario: Job page has a job-specific title and canonical

- **WHEN** `GET /jobs/:slug` is requested for an existing job
- **THEN** the HTML `<head>` contains a `<title>` built from the job's title and
  company, a `<meta name="description">`, a `<link rel="canonical">` to the
  job's canonical URL, and Open Graph tags

#### Scenario: Job page emits a large-image preview

- **WHEN** `GET /jobs/:slug` is requested for an existing job
- **THEN** the HTML `<head>` contains an `og:image` whose absolute URL ends in
  `/jobs/:slug/og.png`, with `og:image:width` 1200 and `og:image:height` 630,
  and a `twitter:card` of `summary_large_image`

#### Scenario: List pages carry their own metadata

- **WHEN** a public list page (`/jobs`, `/companies`) is requested
- **THEN** its `<head>` carries a page-appropriate title, description, and
  canonical URL distinct from the job-detail metadata, and — having no preview
  image — keeps the `summary` Twitter Card with no `og:image`
