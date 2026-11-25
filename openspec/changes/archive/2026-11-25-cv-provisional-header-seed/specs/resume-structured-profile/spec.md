## ADDED Requirements

### Requirement: Provisional contacts are the identity source for CV seed while extract is pending

Contact fields recovered as provisional identity on the cookie résumé read (from a superseded structured blob while the structure stamp is stale or missing) SHALL be the same identity source CV seed and empty-header heal paths use. Those paths MUST treat provisional contacts as identity only: they MUST NOT treat a superseded blob as a current structured résumé for semantic sections. The stamp-gated `Structured` read used for “is the file-owned parse current?” remains false while pending; seed composition layers provisional contacts on top of that gate rather than flipping the stamp.

#### Scenario: Seed and profile see the same provisional name

- **WHEN** a newer résumé has been uploaded, extraction has not completed, and a superseded blob still holds a full name
- **THEN** both the résumé status read and a usable CV seed composition carry that full name as contact identity

#### Scenario: Stamp gate stays false while contacts are provisional

- **WHEN** the structure stamp does not match the current upload
- **THEN** the current-structure read still reports absent/stale even though provisional contacts are available for identity
