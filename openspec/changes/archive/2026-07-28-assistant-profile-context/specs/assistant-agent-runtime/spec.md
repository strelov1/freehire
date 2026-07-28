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
document and applying a CV patch. Rendering the CV SHALL NOT be a tool: the
workspace already previews the document beside the chat, so a render would return
bytes the model cannot read and the user a copy of what is on screen. Each tool
SHALL declare a
JSON schema for its arguments and return structured data, not human-formatted
text. Moderator-only operations (job authoring, submission review) SHALL NOT be
exposed.

#### Scenario: Search results carry the data needed to recommend

- **WHEN** the model calls `search_jobs`
- **THEN** the result contains each hit's structured fields including its `public_slug` and its full description, so the model can screen the set without a follow-up call per hit

#### Scenario: An action tool changes the caller's state

- **WHEN** the model calls the apply tool for a vacancy on the user's behalf
- **THEN** the vacancy is recorded as applied for that user, exactly as the equivalent HTTP endpoint would record it, and the result reports the new state

#### Scenario: Moderator operations are unavailable

- **WHEN** a session is created for a user who is also a moderator
- **THEN** no job-authoring or submission-review tool is offered to the model

## ADDED Requirements

### Requirement: The agent reads the saved profile instead of interrogating the user

The agent SHALL be able to read the calling user's saved job-search profile
through a `get_profile` tool available in every session, and its prompt SHALL
instruct it to do so before asking the user what they are looking for. The result
carries the profile's specializations, skills, excluded skills and location
preferences, together with the caller's structured CV projected onto its
contact-free view — `full_name`, `email`, `phone` and `links` SHALL NOT appear in
it, because a tool result is persisted in the transcript and replayed into the
model's context on every later turn.

A caller who has never saved a profile SHALL receive a result, not an error, and
that result SHALL direct the agent to send the user to the profile page rather
than reconstruct the same preferences through questions in the conversation — the
profile persists and drives the user's recommendations, where chat answers do not.

#### Scenario: The agent grounds a search in the saved profile

- **WHEN** a signed-in user with a saved profile opens a conversation and asks for help finding work
- **THEN** the agent calls `get_profile`, searches from the roles, skills and location preferences it returns, and asks only about what the profile does not answer

#### Scenario: The profile result carries no contact details

- **WHEN** the model calls `get_profile` for a user whose structured CV holds a name, email, phone and links
- **THEN** the result carries the CV's professional content — headline, years, experience, education, skills — and none of those four contact fields

#### Scenario: A user with no profile is sent to fill one in

- **WHEN** the model calls `get_profile` for a user who has never saved one
- **THEN** the tool returns successfully, reporting that no profile exists and that the user should be pointed at the profile page, rather than failing or returning an empty profile the model might treat as "no preferences"
