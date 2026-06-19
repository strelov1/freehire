## ADDED Requirements

### Requirement: ATS detection from page HTML

The system SHALL provide a pure function that detects a supported ATS board from a
page's HTML, returning the provider and board slug. It SHALL recognise Greenhouse
(`boards.greenhouse.io/<slug>` and the `embed/job_board?for=<slug>` variant), Lever
(`jobs.lever.co/<slug>`), and Ashby (`jobs.ashbyhq.com/<slug>`). The detected slug
SHALL be validated to a sane shape (lowercase alphanumeric and hyphens); a match
that does not yield such a slug SHALL be treated as no detection. When several
providers' links are present, the function SHALL return a single deterministic
result. The function SHALL perform no I/O.

#### Scenario: Direct Greenhouse board link

- **WHEN** HTML contains `https://boards.greenhouse.io/acme`
- **THEN** detection returns provider `greenhouse` and slug `acme`

#### Scenario: Greenhouse embed link

- **WHEN** HTML contains `boards.greenhouse.io/embed/job_board?for=acme`
- **THEN** detection returns provider `greenhouse` and slug `acme` (not `embed`)

#### Scenario: Lever and Ashby links

- **WHEN** HTML contains `https://jobs.lever.co/acme` (or `https://jobs.ashbyhq.com/acme`)
- **THEN** detection returns provider `lever` (or `ashby`) and slug `acme`

#### Scenario: No ATS link

- **WHEN** HTML contains no supported ATS URL
- **THEN** detection returns ok = false

### Requirement: Unmatched-company extraction from collection datasets

The system SHALL provide an extract step that reads the collection datasets (using
the dataset URLs from the collections registry as the single source of truth for
their locations), parses each company's name and website, and emits only the
companies whose normalized-name slug is **absent** from a supplied set of existing
company slugs. A company with no website SHALL be omitted. The output SHALL pair
each emitted company's name with its website so the resolve step can fetch it.

#### Scenario: A company already in the catalogue is dropped

- **WHEN** a dataset company's normalized-name slug is present in the supplied
  company-slug set
- **THEN** it is not emitted by the extract step

#### Scenario: An unmatched company with a website is emitted

- **WHEN** a dataset company's slug is absent from the set and it has a website
- **THEN** it is emitted with its name and website

#### Scenario: A company without a website is omitted

- **WHEN** a dataset company has no website
- **THEN** it is not emitted (there is nothing to follow)

### Requirement: Website-to-board resolution writes per-provider seeds

The system SHALL provide a resolve step that, for each input company, fetches a
small fixed set of candidate career pages (the homepage, common careers/jobs paths,
and a careers/jobs link discovered on the homepage) through the shared HTTP client,
runs ATS detection on each, and stops at the first page that yields a board. It
SHALL accumulate the detected slugs per provider and write one seed file per
provider (the input format the existing `harvest-boards` consumes). The run SHALL
be best-effort: a failure fetching or parsing one company (timeout, bot-block,
missing careers page, JS-only page) SHALL be logged and skipped without aborting
the run. Resolution SHALL NOT write to `sources/*.yml` directly — the per-provider
seeds feed `harvest-boards`, which validates each slug against the provider API
before any board is committed.

#### Scenario: A resolved company contributes to its provider's seed

- **WHEN** a company's careers page links to `jobs.lever.co/acme`
- **THEN** `acme` appears in the lever seed file

#### Scenario: A failed company does not abort the run

- **WHEN** one company's site times out or has no detectable ATS
- **THEN** it is skipped and logged, and the remaining companies are still processed

#### Scenario: Seeds feed the existing validation step

- **WHEN** the resolve step finishes
- **THEN** it has written per-provider seed files (no `sources/*.yml` writes), which
  `harvest-boards <provider> <seed>` then validates and commits
