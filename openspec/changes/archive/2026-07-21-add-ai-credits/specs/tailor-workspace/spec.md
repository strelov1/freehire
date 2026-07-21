## ADDED Requirements

### Requirement: Creating a tailored CV debits points

The system SHALL charge **3 points** (configurable, `CREDITS_COST_TAILOR`)
against the caller's unified points balance (see the `ai-credits` capability)
when a new tailored CV is created via `POST /api/v1/me/cvs/tailor`. The debit is
idempotent by `cv_id`: creating the tailored CV consumes points once, while
re-opening or resuming that CV's tailoring session MUST NOT debit again. When the
caller's remaining points are below the tailor cost, the system SHALL respond
`HTTP 402` with `error`, `remaining`, and `resets_at`, and MUST NOT create the
tailored CV or mint a tailoring session.

#### Scenario: Bootstrapping a tailored CV consumes points

- **WHEN** a user with at least the tailor cost remaining creates a new tailored CV
- **THEN** the tailored CV is created, a tailoring session is minted, and the tailor cost is debited from the caller's balance

#### Scenario: Insufficient points blocks tailoring

- **WHEN** a user whose remaining points are below the tailor cost attempts to create a tailored CV
- **THEN** the system responds `402` with `remaining` and `resets_at`, and no tailored CV or session is created

#### Scenario: Resuming an existing tailored CV is free

- **WHEN** a user re-opens or resumes the tailoring session of a tailored CV they already created
- **THEN** no additional points are consumed
