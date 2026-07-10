# dataart-source Specification

## Purpose
TBD - created by archiving change dataart-source. Update Purpose after archive.
## Requirements
### Requirement: DataArt careers crawl

The system SHALL provide a `dataart` source adapter that crawls DataArt's careers
site (`www.dataart.team`) into the catalogue. It is a **boardless single-company**
adapter: config entries carry no board, the host is fixed, and it is excluded from
the source facet. The crawl is keyless. It enumerates
`https://www.dataart.team/sitemap.xml`, keeps the canonical English vacancy URLs,
fetches each vacancy page, and maps its schema.org `JobPosting` ld+json to a `Job`.

#### Scenario: Sitemap enumerates, each vacancy maps to a Job

- **WHEN** the adapter fetches its configured company entry
- **THEN** it returns one `Job` per canonical English vacancy URL in the sitemap,
  with the posting's title, sanitized HTML description, joined `jobLocation`
  city/country text, apply/detail URL, and posted-at date read from the vacancy
  page's `JobPosting` ld+json

#### Scenario: Stable dedup identity from the vacancy code

- **WHEN** the adapter maps a vacancy URL `https://www.dataart.team/vacancies/{code}`
- **THEN** the `Job.ExternalID` is `{code}`, so re-crawling dedups to the same
  catalogue row

#### Scenario: Each vacancy ingested once across localisations

- **WHEN** the sitemap contains both the canonical `/vacancies/{code}` URL and its
  localised variants (`/es/vacancies/{code}`, `/ua/...`, `/pl/...`, `/bg/...`) and
  the listing root `/vacancies`
- **THEN** only the canonical English URL is crawled; the localised variants and
  the listing root are skipped, so a vacancy is not ingested multiple times

#### Scenario: A vacancy page without a JobPosting is skipped

- **WHEN** a vacancy page fetch fails or the page carries no `JobPosting` ld+json
- **THEN** that vacancy is skipped and the rest of the crawl still returns their
  Jobs

