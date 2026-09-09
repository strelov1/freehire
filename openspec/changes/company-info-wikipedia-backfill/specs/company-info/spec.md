## ADDED Requirements

### Requirement: A run-once worker fills tagline gaps by matching companies to Wikipedia

The system SHALL provide a run-once host worker that, for every company whose
stored `tagline` is NULL or empty, looks up the company by its display name
against Wikipedia and, when a match passes the confidence gate defined below,
writes a `tagline` and a `company_info` summary field derived from that
match's own summary text, and sets `company_info_at`.

A company that already has a non-empty `tagline` (written by this worker or
any other company-info source) SHALL NOT be looked up or modified.

The worker SHALL NOT read or write `job_count`, `collections`, or any
job-derived facet column (`company_types`, `company_sizes`, `countries`,
`domains`, `regions`).

#### Scenario: A company with no tagline receives one from a confident match

- **WHEN** the worker processes a company with an empty `tagline` and its name
  resolves to a Wikipedia article confidently typed as a company or
  organization
- **THEN** the company's `tagline` and `company_info` summary field are set
  from that article, and `company_info_at` is updated

#### Scenario: A company that already has a tagline is left untouched

- **WHEN** the worker encounters a company whose `tagline` is already
  non-empty
- **THEN** it performs no lookup and makes no write for that company

#### Scenario: Job-derived facets are never touched

- **WHEN** the worker fills a company's `tagline`
- **THEN** the company's `job_count`, `collections`, and job-derived facet
  columns are unchanged

### Requirement: A match is accepted only when confidently typed as a company or organization

A candidate Wikipedia/Wikidata match SHALL be accepted only when the matched
entity's own type classification places it within a curated set of
business/organization types (for example: company, corporation, public
company, business, bank, or a narrower subtype of one of these). A match
whose type falls outside that set, or whose type cannot be determined, SHALL
be rejected, and the worker SHALL make no write for that company.

The type check SHALL NOT rely on keyword-matching the entity's
natural-language description or summary text, because that approach both
rejects valid matches whose description omits a recognized keyword and
accepts invalid matches whose description happens to contain one.

#### Scenario: A same-named person, place, or concept is rejected

- **WHEN** a company's name resolves to a Wikipedia article whose subject is
  typed as a person, a place, a military unit, an abstract concept, or a
  disambiguation page
- **THEN** the worker rejects the match and writes nothing for that company

#### Scenario: A valid company match is accepted regardless of description wording

- **WHEN** a company's name resolves to an article typed as a business or
  organization, even if its description text uses wording outside any fixed
  keyword list (for example "defense contractor" or "electrical contractor")
- **THEN** the worker accepts the match and fills the company's `tagline`

### Requirement: The backfill is idempotent and safe to interrupt or re-run

Re-running the worker over the same company data and the same Wikipedia
content SHALL produce the same stored `tagline`/`company_info` values, with
no duplicate rows and no changes to companies already filled by this or any
other source. Stopping the worker partway through a run SHALL leave already
-written companies unaffected and lose no completed work.

#### Scenario: Re-running after a full pass changes nothing

- **WHEN** the worker is run a second time after a first run has already
  filled the eligible companies it could confidently match
- **THEN** the second run performs no lookups against companies that already
  have a `tagline` and writes nothing

#### Scenario: An interrupted run resumes without redoing completed writes

- **WHEN** the worker is stopped after filling some companies and started
  again
- **THEN** it does not re-write the companies already filled, and continues
  with the companies still missing a `tagline`
