## ADDED Requirements

### Requirement: An adapter may supply a company-level description separate from posting bodies

A `Source` adapter MAY, when its platform exposes a company/board-level
description through a distinct endpoint or field from the one it uses for
postings, fetch and return that text once per board crawl. An adapter that
does not implement this SHALL simply return nothing for it, and the pipeline
SHALL treat that as "no company description available" rather than an error.

Fetching a company-level description SHALL NOT require a per-posting
request: it is fetched at most once per board per crawl, independent of how
many postings that board yields.

#### Scenario: An adapter returns a company description alongside its postings

- **WHEN** an adapter's platform exposes a company-level description and the
  adapter implements this capability
- **THEN** a single crawl of that board yields the postings as before, plus
  one company-level description for the board

#### Scenario: An adapter without this capability yields nothing extra

- **WHEN** an adapter does not implement a company-level description lookup
- **THEN** the pipeline receives no company description for that board and
  proceeds exactly as it did before this capability existed

#### Scenario: A blank company-level field is not treated as a description

- **WHEN** an adapter's platform exposes the field but the employer left it
  empty
- **THEN** the adapter returns nothing for it, not an empty string
