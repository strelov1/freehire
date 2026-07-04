# company-og-images Specification

## Purpose
TBD - created by archiving change og-brand-cards. Update Purpose after archive.
## Requirements
### Requirement: Per-company Open Graph preview image endpoint

The web tier SHALL expose `GET /companies/:slug/og.png` that returns a 1200×630
PNG preview image rendered for the company identified by `:slug`. The endpoint
SHALL resolve the company through the same read path as the company-detail page
(by slug). When no company exists for the slug, the endpoint SHALL respond `404`
(no image). The response SHALL carry `Content-Type: image/png` and a
cache-friendly `Cache-Control` header so crawlers and caches can reuse it.

#### Scenario: Existing company renders a PNG

- **WHEN** `GET /companies/:slug/og.png` is requested for an existing company
- **THEN** the response status is `200`, the `Content-Type` is `image/png`, and
  the body is a valid PNG (begins with the PNG signature)

#### Scenario: Unknown slug returns 404

- **WHEN** `GET /companies/:slug/og.png` is requested for a slug with no company
- **THEN** the response status is `404` and no image body is returned

#### Scenario: Response is cacheable

- **WHEN** the endpoint returns an image
- **THEN** the response includes a `Cache-Control` header marking it publicly
  cacheable with a finite max-age

### Requirement: Company card content

The rendered card SHALL present the company name and a hero-sized company logo,
its live open-jobs count phrased as human text ("N open jobs", singular/plural
correct, thousands-grouped), and, when present, the company tagline and chips for
the first industry and the HQ country. The HQ country SHALL be shown as a
readable country name (via the shared country-label helper), not an emoji flag.
The card SHALL carry the freehire wordmark and the site domain.

#### Scenario: Company with full data shows name, count, tagline, chips

- **WHEN** a company has a tagline, at least one industry, and an HQ country
- **THEN** the card shows the logo, name, "N open jobs", the tagline, and a chip
  for the first industry and the HQ country name, plus the freehire wordmark

#### Scenario: Open-jobs count reflects the live total

- **WHEN** the endpoint renders a company card
- **THEN** the count matches the company's current open-jobs total from the same
  search read path the company-detail page uses

### Requirement: Company card logo and sparse-data degradation

The card SHALL show the company logo resolved from the logo.dev name endpoint,
fetched server-side and embedded as image bytes. When the logo cannot be
resolved (not found, timeout, or any error), the card SHALL fall back to a
neutral tile bearing a one or two letter monogram derived from the company name;
logo resolution failure SHALL NOT fail the image render. Absent optional fields
(tagline, industry, HQ) SHALL be omitted entirely rather than rendered as empty
or placeholder content, and no literal null/undefined SHALL appear.

#### Scenario: Logo missing falls back to a monogram

- **WHEN** logo.dev returns no logo (404), times out, or errors
- **THEN** the card shows a monogram tile derived from the company name and the
  image still renders `200`

#### Scenario: Missing optional fields keep a clean card

- **WHEN** a company has no tagline, no industry, and no HQ country
- **THEN** the card renders cleanly with the logo, name, and open-jobs count, and
  shows no empty chips and no literal null text

