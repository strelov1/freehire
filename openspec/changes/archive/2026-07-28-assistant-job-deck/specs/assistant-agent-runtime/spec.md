## MODIFIED Requirements

### Requirement: The tool surface mirrors the CLI's job-seeker commands

The agent SHALL be given typed tools covering the same operations the `freehire`
CLI exposes to a job seeker: reading the caller's own saved job-search profile
(`get_profile`), reading the filter vocabulary (`facets`), searching
vacancies with keyword and facet filters (`search_jobs`), reading one vacancy
(`get_job`) or company (`get_company`), scoring a skill set against the market
(`market_fit`), saving, unsaving and applying to a vacancy, setting an
application stage or note (`track_job`), listing the caller's tracked jobs
(`my_jobs`), and — in a tailoring session — reading the tailoring context and CV
document and applying a CV patch. It SHALL additionally be given `present_jobs`,
the one tool whose purpose is presentation rather than retrieval or state change:
it is how a vacancy reaches the user's screen. Rendering the CV SHALL NOT be a
tool: the workspace already previews the document beside the chat, so a render
would return bytes the model cannot read and the user a copy of what is on
screen. Each tool SHALL declare a
JSON schema for its arguments and return structured data, not human-formatted
text. Moderator-only operations (job authoring, submission review) SHALL NOT be
exposed.

#### Scenario: Search results carry the data needed to recommend

- **WHEN** the model calls `search_jobs`
- **THEN** the result contains each hit's structured fields including its `public_slug` and its full description, so the model can screen the set without a follow-up call per hit

#### Scenario: An action tool changes the caller's state

- **WHEN** the model calls the apply tool for a vacancy on the user's behalf
- **THEN** the vacancy is recorded as applied for that user, exactly as the equivalent HTTP endpoint would record it, and the result reports the new state

#### Scenario: A presentation tool changes nothing

- **WHEN** the model calls `present_jobs`
- **THEN** no user state is written; the call only validates the submitted slugs and reports which of them will be shown

#### Scenario: Moderator operations are unavailable

- **WHEN** a session is created for a user who is also a moderator
- **THEN** no job-authoring or submission-review tool is offered to the model
