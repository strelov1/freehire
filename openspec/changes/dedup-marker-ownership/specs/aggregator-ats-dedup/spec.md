## MODIFIED Requirements

### Requirement: Suppression fails over when the ATS twin closes

The system SHALL re-evaluate suppression on each reindex run so that a suppressed
aggregator posting whose ATS twin has closed is un-suppressed (its `duplicate_of` cleared)
and re-enters search, embedding, and enrichment. Re-evaluation SHALL be idempotent — a
run that changes no relationships performs no writes. Suppression and release SHALL be
expressed exclusively through `duplicate_of_aggregator`, which no other pass writes, so a
suppression is cleared only by this pass deciding the relationship ended.

#### Scenario: Closed ATS twin releases the aggregator copy

- **WHEN** the ATS posting that suppressed an aggregator copy is closed and the reindex
  runs again
- **THEN** the aggregator copy's `duplicate_of` is cleared and it becomes eligible for
  search, embedding, and enrichment again

#### Scenario: A no-change run writes nothing

- **WHEN** the suppression pass runs and every aggregator/ATS relationship is already
  correct
- **THEN** no `duplicate_of` values are written

#### Scenario: Another pass cannot release a suppression

- **WHEN** the role-cluster recompute or the fuzzy pass runs over a suppressed aggregator
  posting and finds it canonical by its own criteria
- **THEN** the suppression stands, the posting stays out of search, embedding, and
  enrichment, and only this pass may release it
