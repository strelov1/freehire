# search-drain-fairness Specification

## Purpose
TBD - created by archiving change search-drain-fairness. Update Purpose after archive.
## Requirements
### Requirement: Fair search queue draining
The worker SHALL alternate one deletion wave and one indexing wave, deletion first. It SHALL preserve independent per-queue MaxPerRun limits and count all claimed entries, including deferred entries, against those limits. A queue returning fewer entries than requested SHALL wait for the next scheduled invocation after that wave; a nonempty sibling queue SHALL continue within its budget.

#### Scenario: Both queues have a backlog
- **WHEN** both queues contain multiple full waves
- **THEN** an indexing wave runs between consecutive deletion waves
- **AND** a deletion wave runs between consecutive indexing waves

#### Scenario: Indexing receives a trickle
- **WHEN** an indexing wave is smaller than the requested batch
- **THEN** the current invocation does not claim another indexing wave
- **AND** pending removals continue within their own budget

#### Scenario: A wave is deferred
- **WHEN** an operation times out without completing its entries
- **THEN** those claimed entries still consume the queue run budget
- **AND** per-item fallback does not spend their attempts

