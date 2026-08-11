## ADDED Requirements

### Requirement: Owner GET of a CV may persist a keep-first header heal

When an authenticated owner reads a single CV they own (`GET /api/v1/me/cvs/:id`) and at least one header contact field on that document is empty, the system SHALL fill those empty fields from the three-layer résumé identity table (owned contacts, else current extract, else provisional contacts) and SHALL persist the healed document when anything changed. Non-empty header fields MUST stay put. Body sections, title, template, and typography MUST NOT change.

This write on GET is justified only as repair of blank identity left by an earlier seed that lacked contacts: a reload must not keep showing an empty name the system already knows. List (`GET /api/v1/me/cvs`), PDF render, and unauthenticated or non-owner reads MUST NOT persist a heal.

#### Scenario: Empty tailored header is filled on owner GET

- **WHEN** the owner GETs a CV whose header is empty and owned or provisional contacts include a name
- **THEN** the returned document's header carries that name and the stored row is updated

#### Scenario: Partial header keeps the name and fills email

- **WHEN** the owner GETs a CV that already has a name but empty email, and identity includes both
- **THEN** the name is unchanged, the empty email is filled, and the stored row is updated

#### Scenario: A second GET does not write again

- **WHEN** the owner GETs the same CV after a heal already filled every empty identity field that identity can supply
- **THEN** the stored document is unchanged (no new revision)

#### Scenario: List does not heal

- **WHEN** the owner lists CVs and a listed CV still has an empty header
- **THEN** the list response does not persist a header heal
