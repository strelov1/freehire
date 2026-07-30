## MODIFIED Requirements

### Requirement: An aggregator posting is suppressed when a first-party ATS twin exists

The system SHALL, when the reindex recomputes duplicate markers, mark an open posting
from an aggregator source as `duplicate_of` an open posting from a non-aggregator (ATS)
source when both share the
same `company_slug`, the same normalized title, and a compatible country. Two postings
have a compatible country when their `countries` arrays overlap, or when either array is
empty. The ATS posting SHALL remain canonical (its `duplicate_of` stays NULL); the
aggregator posting SHALL become the duplicate. A source is an aggregator when its
provider is in `sources.AggregatorProviders()`.

Titles match on either of two keys. The **plain key** is lowercase with runs of
non-alphanumeric characters collapsed to a single space. The **decorated key** additionally
decodes HTML entities and drops a trailing clause introduced by ` - `, ` — `, ` | ` or `:` —
the ways an aggregator appends technologies or a team to an otherwise identical title. A
posting matches when either key is equal and non-empty.

The decorated key MUST NOT strip a parenthetical group or a clause after a comma. Both carry
meaning rather than decoration: at one company `Senior Software Engineer, Backend (Traffic)`,
`(Payments)`, `(Identity)` and `(Infrastructure)` are separate roles, as are
`Senior Software Engineer, Backend` and `Senior Software Engineer, Fullstack`. Stripping
either would merge distinct jobs.

#### Scenario: Aggregator copy of an ATS job is suppressed

- **WHEN** a company has an open ATS posting and an open aggregator posting with the same
  normalized title and overlapping (or empty) countries
- **THEN** the aggregator posting's `duplicate_of` points to the ATS posting, and the ATS
  posting stays canonical

#### Scenario: A trailing colon clause is decoration

- **WHEN** an aggregator posting is titled `Senior Software Engineer: Full-Stack with TypeScript`
  and the company has an ATS posting titled `Senior Software Engineer`
- **THEN** the aggregator posting is suppressed as a duplicate of it

#### Scenario: A parenthetical distinguishes roles and is kept

- **WHEN** an aggregator posting titled `Senior Software Engineer, Backend (Traffic)` and an ATS
  posting titled `Senior Software Engineer, Backend (Payments)` exist at one company
- **THEN** neither is suppressed by this pass

#### Scenario: A clause after a comma distinguishes roles and is kept

- **WHEN** an aggregator posting titled `Senior Software Engineer, Backend` and an ATS posting
  titled `Senior Software Engineer, Fullstack` exist at one company
- **THEN** neither is suppressed by this pass

#### Scenario: Same title in a different country is not suppressed

- **WHEN** an aggregator posting and an ATS posting of the same company share a title but
  their non-empty `countries` arrays do not overlap
- **THEN** neither posting is suppressed by this pass

#### Scenario: An ATS posting is never demoted by an aggregator twin

- **WHEN** an aggregator posting and an ATS posting match on company, title, and country
- **THEN** only the aggregator posting may be marked `duplicate_of`; the ATS posting is
  never marked a duplicate of the aggregator posting

#### Scenario: Two aggregator postings are not merged by this pass

- **WHEN** two postings of the same company and title both come from aggregator sources
  and no ATS twin exists
- **THEN** this pass suppresses neither (cross-aggregator collapse stays the role-cluster
  pass's responsibility)
