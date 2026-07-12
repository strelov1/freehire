## ADDED Requirements

### Requirement: An aggregator posting is suppressed when a first-party ATS twin exists

The system SHALL, when the reindex recomputes duplicate markers, mark an open posting
from an aggregator source as `duplicate_of` an open posting from a non-aggregator (ATS)
source when both share the
same `company_slug`, the same normalized title, and a compatible country. Two postings
have a compatible country when their `countries` arrays overlap, or when either array is
empty. The ATS posting SHALL remain canonical (its `duplicate_of` stays NULL); the
aggregator posting SHALL become the duplicate. A source is an aggregator when its
provider is in `sources.AggregatorProviders()`.

Normalized title equality is lowercase with runs of non-alphanumeric characters collapsed
to a single space (the same normalization the role-cluster pass uses), with no suffix
stripping and no HTML-entity decoding.

#### Scenario: Aggregator copy of an ATS job is suppressed

- **WHEN** a company has an open ATS posting and an open aggregator posting with the same
  normalized title and overlapping (or empty) countries
- **THEN** the aggregator posting's `duplicate_of` points to the ATS posting, and the ATS
  posting stays canonical

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

### Requirement: A suppressed aggregator posting is hidden from active surfaces but reachable by link

The system SHALL treat a suppressed aggregator posting exactly as any other
`duplicate_of` row: excluded from job search / the Meilisearch index, excluded from
semantic embedding with any existing vector removed, and excluded from LLM enrichment,
while remaining stored and served on its public detail page by slug and listed among a
role cluster's copies.

#### Scenario: Suppressed copy leaves search, embedding, and enrichment

- **WHEN** an aggregator posting becomes `duplicate_of` an ATS posting
- **THEN** it is not returned by job search, not enqueued for embedding (its vector is
  removed), and not enqueued for LLM enrichment

#### Scenario: Suppressed copy is still reachable by its slug

- **WHEN** a client opens the public detail URL of a suppressed aggregator posting
- **THEN** the posting is served

### Requirement: Suppression fails over when the ATS twin closes

The system SHALL re-evaluate suppression on each reindex run so that a suppressed
aggregator posting whose ATS twin has closed is un-suppressed (its `duplicate_of` cleared)
and re-enters search, embedding, and enrichment. Re-evaluation SHALL be idempotent — a
run that changes no relationships performs no writes.

#### Scenario: Closed ATS twin releases the aggregator copy

- **WHEN** the ATS posting that suppressed an aggregator copy is closed and the reindex
  runs again
- **THEN** the aggregator copy's `duplicate_of` is cleared and it becomes eligible for
  search, embedding, and enrichment again

#### Scenario: A no-change run writes nothing

- **WHEN** the suppression pass runs and every aggregator/ATS relationship is already
  correct
- **THEN** no `duplicate_of` values are written
