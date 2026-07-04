## ADDED Requirements

### Requirement: Static brand Open Graph image

The web tier SHALL ship a committed static 1200×630 PNG brand image at
`web/static/og.png` (served at `/og.png`). The image SHALL carry the freehire
brand mark and wordmark, the headline "Open source job aggregator that covers all
ATS", and a stat-strip of the catalogue scale (open-jobs, companies, and
ATS-platform figures). The stat figures SHALL be fixed constants baked into the
image at generation time (not fetched at request time).

#### Scenario: Brand image is a valid PNG at a stable path

- **WHEN** `GET /og.png` is requested
- **THEN** the response is a valid 1200×630 PNG (begins with the PNG signature)

#### Scenario: Brand image is regenerable from source

- **WHEN** the brand-image generator script is run
- **THEN** it renders `web/static/og.png` from the shared OG render core using
  the freehire fonts and mark, with the headline and stat-strip as specified

### Requirement: Default Open Graph image for pages without their own

Every page's rendered document metadata SHALL emit an `og:image` (and the
matching `twitter:image` with a `summary_large_image` card). A page that does
not supply a page-specific preview SHALL default to the absolute URL of the
static brand image (`<origin>/og.png`). A page that supplies its own preview
(e.g. a job or company card) SHALL override the default with its own image.

#### Scenario: Generic page falls back to the brand image

- **WHEN** a page renders without passing a page-specific preview image
- **THEN** its `og:image` and `twitter:image` resolve to `<origin>/og.png` and
  the Twitter card is `summary_large_image`

#### Scenario: Page-specific image overrides the default

- **WHEN** a page supplies its own preview image
- **THEN** its `og:image` is that image, not the brand default
