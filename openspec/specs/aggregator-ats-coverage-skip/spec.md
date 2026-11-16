# Aggregator posting retention specification

## Purpose

Preserve vacancies that a company publishes through different channels. A different vacancy
from the same employer is not evidence that a source posting is a duplicate.

## Requirements

### Requirement: Company coverage never discards or closes a posting

The ingest runner SHALL save each readable, catalogue-eligible aggregator posting regardless
of other vacancies stored for the employer. A re-listed stored posting SHALL take the normal
liveness refresh path. Buffered and streaming ingestion SHALL follow the same rule.

#### Scenario: Another channel has an unrelated vacancy

- **GIVEN** Cloud.ru has a frontend vacancy from Telegram
- **WHEN** Habr Career lists a pentester vacancy at Cloud.ru
- **THEN** ingest saves the pentester vacancy instead of dropping or closing it as covered

### Requirement: Posting-level duplicate detection remains active

The existing identity, role fingerprint, aggregator/ATS and fuzzy duplicate mechanisms SHALL
continue to compare individual postings. Removing company-wide suppression SHALL NOT clear
any duplicate markers.

### Requirement: Hydration retains catalogue filtering and sweep guards

Adapters may avoid hydrating a posting rejected by the catalogue filter. They SHALL NOT skip
its body merely because another vacancy exists at that employer. Unreadable details and
interrupted crawls retain their existing liveness and stale-close safeguards.

### Requirement: Recovery distinguishes confirmed vacancies from discovery records

A repair of historical `ats_covered` closures SHALL preserve duplicate markers and actual
source timestamps, reject expired deadlines, and queue reopened records for search. Records
without recent source evidence require another crawl. Unpublished LinkedIn native-discovery
records SHALL NOT be reopened as public vacancies by this repair.
