# dict-gap-reporting Specification

## Purpose

Surfaces where the deterministic skill-tagging and title-classification dictionaries miss
signal that LLM enrichment already captured, by ranking candidate gaps for a human curator
to review — without ever writing to the dictionaries or the database itself.

## Requirements

### Requirement: Skill gap candidates rank unresolved enrichment skills by frequency

The system SHALL compare the raw skill phrases recorded in job enrichment against the
deterministic skill-tagging dictionary and report, ranked by frequency, every phrase the
dictionary resolves to no canonical skill.

#### Scenario: A frequently-mentioned skill with no dictionary entry surfaces
- **WHEN** an enrichment-recorded skill phrase resolves to no canonical skill in the
  skill-tagging dictionary, and it was recorded on multiple postings
- **THEN** the report includes that phrase with its occurrence count, and phrases with
  higher counts are ranked above phrases with lower counts

#### Scenario: A skill the dictionary already resolves is excluded
- **WHEN** an enrichment-recorded skill phrase resolves to a canonical skill in the
  skill-tagging dictionary
- **THEN** the report does not include that phrase

#### Scenario: Trivial spelling variants of the same unresolved phrase are collapsed
- **WHEN** two unresolved enrichment skill phrases differ only in letter case, a
  hyphen/underscore used as a word separator, surrounding/internal whitespace, or a
  trailing sentence-punctuation mark
- **THEN** the report treats them as one candidate and sums their occurrence counts

#### Scenario: Symbols that are part of a technology's own name are not collapsed away
- **WHEN** two enrichment skill phrases differ by a symbol that is part of a
  technology's identity rather than incidental formatting (for example "C++" vs.
  "C#" vs. bare "C")
- **THEN** the report treats them as separate candidates, even when one of them
  happens to resolve in the skill-tagging dictionary and the other does not

### Requirement: Classify drift candidates rank titles where the dictionary disagrees with the model

The system SHALL recompute the deterministic seniority and category classification for each
enriched job's title and report, ranked by frequency, every distinct title where the
dictionary's answer disagrees with the value LLM enrichment recorded for that job — tracked
separately for seniority and for category.

#### Scenario: A title with disagreeing seniority surfaces
- **WHEN** recomputing the title-classification dictionary's seniority for a title yields a
  different value than what enrichment recorded for jobs with that title
- **THEN** the report's seniority list includes that title with the dictionary's value, the
  enrichment value, and how many jobs shared that title

#### Scenario: A title with disagreeing category surfaces
- **WHEN** recomputing the title-classification dictionary's category for a title yields a
  different value than what enrichment recorded for jobs with that title
- **THEN** the report's category list includes that title with the dictionary's value, the
  enrichment value, and how many jobs shared that title

#### Scenario: A title where the dictionary and the model agree is excluded
- **WHEN** the title-classification dictionary's recomputed seniority and category both
  match what enrichment recorded for a title
- **THEN** the report excludes that title from both the seniority and category lists

### Requirement: Both reports are read-only and run only by hand

The system SHALL produce both reports without writing to the database or to any dictionary
source file, and SHALL NOT run either report on a schedule.

#### Scenario: Running a report changes nothing in the database
- **WHEN** a curator runs the skill-gap or classify-drift report to completion
- **THEN** no row in the database is inserted, updated, or deleted as a result

#### Scenario: Neither report is wired to automatic execution
- **WHEN** the reports are deployed
- **THEN** no scheduled job or timer invokes either report automatically; each is started
  only by a curator running it directly
