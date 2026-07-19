# clinch-source Specification

## Purpose
TBD - created by archiving change clinch-detail-via-paced-challenge-layer. Update Purpose after archive.
## Requirements
### Requirement: clinch hydrates job descriptions through a paced challenge-aware getter

The clinch adapter SHALL attempt to fetch each posting's detail page through a
paced, challenge-aware HTML getter and populate the job description from the
detail page's `div.job-description` content. The detail fetch SHALL be paced so
the run's aggregate request rate stays under ClinchTalent's per-IP AWS-WAF
challenge window.

#### Scenario: Detail page yields a description

- **WHEN** clinch fetches a posting whose detail page returns HTTP 200 with a `div.job-description`
- **THEN** the resulting job's `Description` is the text of that block
- **AND** the title and location reconstructed from the URL slug are unchanged

#### Scenario: Detail page has no description block

- **WHEN** the detail page returns 200 but contains no `div.job-description`
- **THEN** the job keeps an empty `Description` (the slug-derived title/location still populate)

### Requirement: A WAF trip latches off detail hydration for the rest of the run

When a detail fetch returns a `ChallengeError`, the clinch adapter SHALL stop
attempting further detail fetches for the remainder of that run and emit the
remaining postings with the slug-reconstructed fields only. Hammering a tripped
WAF is both wasteful and prolongs the per-IP penalty, so the latch is one-way
within a run.

#### Scenario: First challenge latches subsequent postings to sitemap-only

- **WHEN** a detail fetch returns a `ChallengeError` partway through a run
- **THEN** clinch stops fetching detail pages for the rest of that run
- **AND** the already-hydrated postings keep their descriptions
- **AND** the remaining postings are emitted with empty `Description` and the slug-derived title/location

### Requirement: clinch is never worse than sitemap-only reconstruction

Adding detail hydration SHALL NOT regress clinch's existing output. Every posting
the sitemap-only adapter produced today SHALL still be produced, with the same
`ExternalID`, `URL`, title, and location; the description is strictly additive.

#### Scenario: All sitemap postings still emitted when detail is unavailable

- **WHEN** every detail fetch fails or challenges
- **THEN** clinch emits exactly the same postings it emits today, each with an empty `Description`

#### Scenario: Detail-fetch failure does not drop the posting

- **WHEN** a single posting's detail fetch errors (non-challenge, e.g. a 404 or timeout)
- **THEN** that posting is still emitted with its slug-derived fields and an empty `Description`
- **AND** the run continues to the next posting

