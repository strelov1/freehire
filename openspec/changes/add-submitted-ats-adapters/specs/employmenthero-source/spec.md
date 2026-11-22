## ADDED Requirements

### Requirement: Resolve an Employment Hero employer board

The system SHALL use the organisation slug from the employer page to identify the board.

#### Scenario: The position metadata names the employer

- **WHEN** a submitted position page links to its Employment Hero organisation
- **THEN** the system resolves the organisation slug as its board

### Requirement: Read the entire jobs listing

The adapter SHALL use the jobs listing to retrieve all positions and their descriptions.

#### Scenario: A later page fails

- **WHEN** the adapter cannot retrieve all positions up to the reported total
- **THEN** it fails the board crawl
