## ADDED Requirements

### Requirement: The skills dictionary covers technologies, not soft skills or domains

The `skills` facet SHALL be derived from a curated **technology** dictionary —
languages, frameworks, datastores, infrastructure, platforms, and engineering
methodologies — and SHALL NOT include soft skills (e.g. communication, leadership)
or industry/domain terms (e.g. retail, nursing), keeping the facet a high-signal
technology filter. The dictionary resolves only known aliases and emits nothing for
what it cannot match (it never guesses).

#### Scenario: A common methodology resolves

- **WHEN** a job description states "we work in an agile environment using scrum"
- **THEN** the derived `skills` include `agile` and `scrum`

#### Scenario: A platform resolves

- **WHEN** a job description mentions "Salesforce and SAP integration experience"
- **THEN** the derived `skills` include `salesforce` and `sap`

#### Scenario: An incidental word does not tag

- **WHEN** a job description says "you will support the rest of the team" but states
  no REST API
- **THEN** the derived `skills` do not include `rest`
