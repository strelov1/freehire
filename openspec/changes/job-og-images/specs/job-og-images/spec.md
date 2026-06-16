## ADDED Requirements

### Requirement: Per-job Open Graph preview image endpoint

The web tier SHALL expose `GET /jobs/:slug/og.png` that returns a 1200×630 PNG
preview image rendered for the job identified by `:slug`, populated from that
job's served wire fields. The endpoint SHALL resolve the job through the same
read path as the job-detail page (by public slug). When no open or closed job
exists for the slug, the endpoint SHALL respond `404` (no image). The response
SHALL carry `Content-Type: image/png` and a cache-friendly `Cache-Control`
header so crawlers and caches can reuse it.

#### Scenario: Existing job renders a PNG

- **WHEN** `GET /jobs/:slug/og.png` is requested for an existing job
- **THEN** the response status is `200`, the `Content-Type` is `image/png`, and
  the body is a valid PNG (begins with the PNG signature)

#### Scenario: Unknown slug returns 404

- **WHEN** `GET /jobs/:slug/og.png` is requested for a slug with no job
- **THEN** the response status is `404` and no image body is returned

#### Scenario: Response is cacheable

- **WHEN** the endpoint returns an image
- **THEN** the response includes a `Cache-Control` header marking it publicly
  cacheable with a finite max-age

### Requirement: Card content from served job fields

The rendered card SHALL present the job's title and company name, and SHALL
include facet chips drawn from the served fields — work mode, seniority, salary,
and skills — in a fixed priority order (work mode, then seniority, then salary,
then skills). Salary SHALL be formatted with the same logic the site uses for
the job-detail salary display (the shared formatter), not a separate
implementation. The card SHALL carry the freehire wordmark and the site domain.

#### Scenario: Full job shows all facets

- **WHEN** a job has a work mode, seniority, salary, and skills
- **THEN** the card shows the title, company, and a chip for each present facet
  in the fixed priority order, plus the freehire wordmark

#### Scenario: Salary uses the shared formatter

- **WHEN** a job carries salary fields
- **THEN** the salary chip text matches the site's shared salary formatter
  output for that job

### Requirement: Graceful degradation for sparse data

The card SHALL remain visually intentional when fields are absent — the common
case in the catalogue. A missing facet SHALL omit its chip entirely rather than
render an empty or placeholder chip; no literal null/undefined SHALL appear. A
title that exceeds the card SHALL shrink within a bounded font-size range,
clamp to at most three lines, and ellipsize the overflow. When more skills exist
than fit, the overflow SHALL collapse to a muted `+N` indicator.

#### Scenario: Missing salary omits the chip

- **WHEN** a job has no salary
- **THEN** the card renders with no salary chip and no empty/placeholder chip in
  its place

#### Scenario: No facets keeps a clean card

- **WHEN** a job has no work mode, seniority, salary, or skills
- **THEN** the card still renders cleanly with the title carrying the
  composition, and shows no empty chips and no literal null text

#### Scenario: Long title is bounded

- **WHEN** a job title is too long for the card at the base font size
- **THEN** the title font shrinks within the allowed range and is clamped to at
  most three lines with an ellipsis

### Requirement: Company logo with monogram fallback

The card SHALL show the company logo resolved from the logo.dev name endpoint
(reusing the publishable token the site already uses), fetched server-side and
embedded as image bytes in the rendered card. When the logo cannot be resolved —
not found, timeout, or any error — the card SHALL fall back to a neutral tile
bearing a one or two letter monogram derived from the company name. Logo
resolution failure SHALL NOT fail the image render.

#### Scenario: Logo found is embedded

- **WHEN** logo.dev returns a logo for the company
- **THEN** the card shows that logo and the image still renders `200`

#### Scenario: Logo missing falls back to a monogram

- **WHEN** logo.dev returns no logo (404), times out, or errors
- **THEN** the card shows a monogram tile derived from the company name and the
  image still renders `200`
