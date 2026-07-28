## MODIFIED Requirements

### Requirement: Create a CV seeded from the stored résumé

When creating a CV, the system SHALL optionally seed its content from the user's **experience bank** for the work history — the banked employments and their publishable atoms — and from the user's `resume_structured` extraction for the sections the bank does not own (contacts, summary, education, languages, links). An atom that is not publishable (`agent_inferred`) MUST NOT be seeded into a CV. When neither the bank nor a structured résumé holds anything, the system SHALL create a valid empty skeleton document. Seeding SHALL NOT modify the stored résumé, the experience bank, or any analysis.

#### Scenario: Seed from the experience bank

- **WHEN** an authenticated user with a populated experience bank issues `POST /api/v1/me/cvs` with seeding requested
- **THEN** the new CV's `Document` work history is pre-filled from the banked employments and their publishable atoms, and contacts, summary, education and languages come from the structured résumé

#### Scenario: Evidence confirmed while tailoring reaches the next CV

- **WHEN** a user who confirmed experience during a tailoring session creates a new CV with seeding requested
- **THEN** that experience appears in the seeded document, even though it was never present in any uploaded résumé

#### Scenario: Unconfirmed evidence is not seeded

- **WHEN** a user whose bank contains `agent_inferred` atoms creates a CV with seeding requested
- **THEN** those atoms are omitted from the seeded document

#### Scenario: Create empty CV when nothing is known

- **WHEN** an authenticated user with an empty bank and no `resume_structured` creates a CV
- **THEN** the system returns a valid empty-skeleton `Document` without error
