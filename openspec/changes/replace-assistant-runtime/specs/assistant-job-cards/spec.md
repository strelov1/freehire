## MODIFIED Requirements

### Requirement: The agent produces unfurlable job links

The assistant's system prompt SHALL direct the agent to present each recommended
or listed vacancy as its canonical `https://freehire.me/jobs/<public_slug>` URL,
one per line, and MUST NOT present the posting's original ATS URL in its place.
The `public_slug` SHALL be carried by the job-search and job-read tool results,
so the agent copies it rather than constructing it from a title.

#### Scenario: Recommending jobs yields cards

- **WHEN** the agent uses its search tool to find vacancies and then recommends them to the user
- **THEN** each recommendation includes the job's `https://freehire.me/jobs/<public_slug>` URL, which the chat renders as a card

#### Scenario: The slug comes from the tool result

- **WHEN** a job-search or job-read tool returns a vacancy
- **THEN** the result includes that vacancy's `public_slug`
