## MODIFIED Requirements

### Requirement: Create a CV seeded from the stored résumé

When creating a CV, the system SHALL optionally seed its content from the user's **experience bank** for work history and portfolio projects, and from the user's `resume_structured` extraction for the sections the bank does not own (contacts, summary, education, languages, skills, certifications, and links). An atom that is not publishable (`agent_inferred`) MUST NOT be seeded into a CV.

Work history SHALL prefer banked job-kind employments and their publishable atoms. When the bank has no employments yet but the current structured résumé still carries experience, the seed MUST fall back to that structure's experience for the work-history section only — the bank remains source of truth once it holds any employment. Portfolio projects SHALL land in the document's projects section with name, optional link, and publishable bullets: banked project-kind employments first when present; otherwise the structured résumé's projects. Certifications from the structured résumé MUST map into the document's certifications section the same way skills map into a skills group.

When neither the bank nor a structured résumé holds anything seedable, the system SHALL create a valid empty skeleton document. Seeding SHALL NOT modify the stored résumé, the experience bank, or any analysis.

#### Scenario: Seed from structured résumé

- **WHEN** an authenticated user with a populated experience bank (jobs and projects) and a `resume_structured` extraction issues `POST /api/v1/me/cvs` with seeding requested
- **THEN** the new CV's experience comes from banked jobs, its projects come from banked projects (including URLs), and contacts, summary, education, languages, skills, certifications, and links come from the structured résumé

#### Scenario: Create empty CV when no résumé exists

- **WHEN** an authenticated user with no seedable bank content and no `resume_structured` creates a CV
- **THEN** the system returns a valid empty-skeleton `Document` without error

#### Scenario: Empty bank falls back to structure experience

- **WHEN** a user whose experience bank has no employments but whose structured résumé lists jobs creates a CV with seeding requested
- **THEN** the seeded document's experience matches the structured résumé's experience

#### Scenario: Structure projects are used when the bank has no projects

- **WHEN** the bank has job employments but no project-kind employments, and the structured résumé lists portfolio projects with links
- **THEN** the seeded document's projects match the structured résumé's projects (name, link, highlights)

#### Scenario: Certifications are seeded

- **WHEN** the structured résumé lists certifications
- **THEN** those certifications appear on the seeded CV document

#### Scenario: Chat-confirmed experience still seeds

- **WHEN** a user who confirmed experience during a tailoring session creates a new CV with seeding requested
- **THEN** that experience appears in the seeded document, even though it was never present in any uploaded résumé

#### Scenario: Unconfirmed evidence is not seeded

- **WHEN** a user whose bank contains `agent_inferred` atoms creates a CV with seeding requested
- **THEN** those atoms are omitted from the seeded document
