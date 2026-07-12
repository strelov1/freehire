## MODIFIED Requirements

### Requirement: JobPosting structured data

The job-detail page SHALL include a `JobPosting` JSON-LD `<script type="application/ld+json">`
block in its server-rendered HTML, populated from the job's public fields
(title, description, hiring organization, location/remote, posting date, and the
application URL), so the posting is eligible for Google Jobs. Company pages SHALL
include `Organization` JSON-LD, and that `Organization` object SHALL carry every
company-info fact the company row provides — its logo, description, homepage and
LinkedIn links (as `sameAs`), founding year, employee count, and HQ country — so
the company reads as a recognizable entity, while omitting any of those fields
the company does not have.

#### Scenario: Job page emits valid JobPosting JSON-LD

- **WHEN** `GET /jobs/:slug` is requested for an existing job
- **THEN** the HTML contains one `application/ld+json` script with `@type`
  `JobPosting` whose `title`, `description`, `hiringOrganization`, and
  `datePosted` reflect the job

#### Scenario: A closed job reflects its status in structured data

- **WHEN** the job carries a `closed_at`
- **THEN** the `JobPosting` data conveys that the posting is no longer accepting
  applications rather than presenting it as open

#### Scenario: Company Organization carries its known company-info facts

- **WHEN** `GET /companies/:slug` is requested for a company whose `company_info`
  provides a logo, description, homepage/LinkedIn links, founding year, employee
  count, and HQ country
- **THEN** the emitted `Organization` JSON-LD includes `logo`, `description`, a
  `sameAs` array holding those links, `foundingDate`, `numberOfEmployees`, and an
  `address` with the country code

#### Scenario: Missing company-info facts are omitted, not emitted empty

- **WHEN** the company has no `company_info` (or only some of its fields)
- **THEN** the `Organization` JSON-LD still emits `name` and `url` and simply
  omits every fact the company lacks (no empty strings, null values, or empty
  arrays)

## ADDED Requirements

### Requirement: Collection landing structured data

The collection landing page (`GET /collections/:slug`) SHALL emit a
`CollectionPage` JSON-LD block whose `mainEntity` is an `ItemList` of the
first page of jobs already rendered on the page. Each `ItemList` entry SHALL be a
summary `ListItem` carrying its 1-based `position`, the job `name`, and the
absolute `url` of the job-detail page — never an embedded full `JobPosting`, so
the list page presents the recommended summary shape rather than many nested
postings.

#### Scenario: Collection page emits CollectionPage with a job ItemList

- **WHEN** `GET /collections/:slug` is requested for a collection that has jobs
- **THEN** the HTML contains an `application/ld+json` script with `@type`
  `CollectionPage` whose `mainEntity` is an `ItemList` whose `itemListElement`
  entries each carry `position`, `name`, and a `url` pointing at `/jobs/:slug`

#### Scenario: An empty collection emits an empty ItemList, not broken JSON-LD

- **WHEN** `GET /collections/:slug` is requested for a collection with no jobs on
  the first page
- **THEN** the `CollectionPage` is still valid JSON-LD with an `ItemList` whose
  `itemListElement` is an empty array
