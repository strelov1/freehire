## MODIFIED Requirements

### Requirement: JobPosting structured data

The job-detail page SHALL include a `JobPosting` JSON-LD `<script type="application/ld+json">`
block in its server-rendered HTML, populated from the job's public fields
(title, description, hiring organization, location/remote, posting date, and the
application URL), so the posting is eligible for Google Jobs. The `JobPosting`
SHALL additionally carry, each only when the underlying data is present: the
hiring organization's `logo`, the job's `skills`, an `experienceRequirements`
derived from the minimum years of experience, and an `educationRequirements`
derived from the required education level. Company pages SHALL include
`Organization` JSON-LD, and that `Organization` object SHALL carry every
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

#### Scenario: JobPosting carries logo, skills, and requirements when present

- **WHEN** `GET /jobs/:slug` is requested for a job that has a company name,
  dictionary skills, a positive minimum years of experience, and a degree-level
  education requirement
- **THEN** the `JobPosting` includes a `hiringOrganization.logo` URL, a `skills`
  value listing those skills, an `experienceRequirements` of type
  `OccupationalExperienceRequirements` whose `monthsOfExperience` equals the
  minimum years × 12, and an `educationRequirements` of type
  `EducationalOccupationalCredential` with a `credentialCategory`

#### Scenario: Absent or signal-free enrichment fields are omitted

- **WHEN** the job has no skills, a minimum experience of zero, and an education
  level of `none`
- **THEN** the `JobPosting` omits `skills`, `experienceRequirements`, and
  `educationRequirements` entirely rather than emitting empty or zero values

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
