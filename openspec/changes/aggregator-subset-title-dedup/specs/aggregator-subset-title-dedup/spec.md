## ADDED Requirements

### Requirement: Suppression also matches when the aggregator title's words are a subset of an ATS title's

The system SHALL suppress an open aggregator posting as `duplicate_of` an open canonical ATS posting
of the same `company_slug` and compatible country when the aggregator title's normalized word set is
a subset of the ATS title's normalized word set, in addition to the existing exact and
entity-decoded/suffix-stripped matches. The aggregator title SHALL have at least two normalized
words for this path to apply. This path is additive: the exact and normalized matches are unchanged.

#### Scenario: An aggregator that dropped middle words matches its ATS twin

- **WHEN** an aggregator posting `Guest Service Agent` and an ATS posting
  `Guest Service Agent - Front Office The St Regis` share a company and compatible country
- **THEN** the aggregator posting is marked `duplicate_of` the ATS posting

#### Scenario: A one-word aggregator title does not match by subset

- **WHEN** an aggregator posting with a single normalized word (e.g. `Chef`) is a subset of many ATS
  titles in the company
- **THEN** the subset path does not suppress it (too generic)

#### Scenario: Exact and normalized behavior is preserved

- **WHEN** an aggregator posting matches an ATS twin on the exact or entity-decoded/suffix-stripped
  key
- **THEN** it is suppressed exactly as before (the subset path only adds matches)

### Requirement: A seniority-only difference does not merge distinct grades

The system SHALL NOT suppress an aggregator posting against an ATS posting whose title adds, over the
aggregator's words, only seniority or qualifier markers (e.g. senior, junior, lead, principal,
staff). The subset match SHALL require at least one added ATS word that is not such a marker.

#### Scenario: Seniority-only difference is not merged

- **WHEN** an aggregator posting `Software Engineer` and an ATS posting `Senior Software Engineer`
  share a company and compatible country, and the only added word is a seniority marker
- **THEN** the aggregator posting is not suppressed by the subset path

#### Scenario: A non-seniority added word does merge

- **WHEN** an aggregator posting `Software Engineer` and an ATS posting `Software Engineer Payments`
  share a company and compatible country
- **THEN** the aggregator posting is marked `duplicate_of` the ATS posting (the aggregator dropped a
  non-seniority word)

### Requirement: Subset-path suppression preserves aggregator-only, canonical-ATS, and failover invariants

The system SHALL apply the subset match under every invariant of the exact suppression: only an
aggregator posting is suppressed (an ATS posting is never demoted), only against an open canonical
ATS posting, only within a compatible country, and a match lost because the ATS twin closed SHALL
release the aggregator copy on the next reindex.

#### Scenario: Country gate holds for the subset path

- **WHEN** an aggregator title is a subset of an ATS title in the same company but their non-empty
  `countries` do not overlap
- **THEN** the aggregator posting is not suppressed

#### Scenario: Subset match releases when the ATS twin closes

- **WHEN** the ATS posting that suppressed an aggregator copy via the subset path closes and the
  reindex runs again
- **THEN** the aggregator copy's `duplicate_of` is cleared and it re-enters search, embedding, and
  enrichment
