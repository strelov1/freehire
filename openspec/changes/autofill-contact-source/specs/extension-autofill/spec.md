## ADDED Requirements

### Requirement: The autofill profile is one canonical contact block

The system SHALL serve the browser extension a fixed set of contact fields assembled
server-side, so the extension holds no rule about where a value comes from: `full_name`,
`first_name`, `last_name`, `email`, `phone`, `location`, `linkedin`, `github` and
`portfolio`. `first_name` and `last_name` SHALL be derived by splitting the full name on
whitespace — the first token is the given name, the remainder the family name. The links
SHALL be sorted by host: the first LinkedIn URL, the first GitHub URL, and the first
remaining URL as the portfolio. A field the sources do not state SHALL be empty rather
than guessed.

This block carries the candidate's identity only. It carries no summary, no work history
and no skills, because it exists to fill the contact section of an application form.

#### Scenario: The block is served with the fields the sources state

- **WHEN** an authenticated caller reads the autofill profile and a source states a name,
  a phone and a LinkedIn URL
- **THEN** the response carries those values, the given and family names split from the
  full name, and empty strings for the fields no source stated

#### Scenario: Links are sorted by host, not by order

- **WHEN** the chosen source states a personal site, then a GitHub URL, then a LinkedIn URL
- **THEN** `linkedin` is the LinkedIn URL, `github` is the GitHub URL, and `portfolio` is
  the personal site

### Requirement: The contact block resolves from an ordered list of sources

The system SHALL resolve the contact block from the first source that answers, in this
order:

1. the user's **base CV** header — the CV that is not a tailored copy;
2. the user's **structured résumé** contact fields;
3. **no source**, leaving every field empty.

A source answers when it exists and states at least one contact value; a source that
exists but states none is passed over, so an empty CV does not silence a résumé that has
the values. The chosen source answers for the **whole** block: fields SHALL NOT be merged
across sources. Merging would restore a value the candidate deliberately removed from
their CV, and the candidate's own copy is the one that must win outright.

The base CV ranks first because it is the copy the candidate authored: the structured
résumé seeds it, and the candidate corrects it afterwards. It is also the only copy the
tailoring agent cannot rewrite — the edit policy denies the agent the header's name, email,
phone and links.

Tailored CVs SHALL NOT be a source at any tier. A tailored copy is written for one vacancy,
and the autofill profile carries no vacancy.

#### Scenario: The base CV answers when it states contacts

- **WHEN** a user has a base CV whose header states their name, and also has a structured
  résumé stating a different name
- **THEN** the block carries the base CV's name

#### Scenario: The structured résumé answers when there is no base CV

- **WHEN** a user has a current structured résumé and no base CV
- **THEN** the block carries the structured résumé's contact fields

#### Scenario: An empty base CV does not silence the structured résumé

- **WHEN** a user has a base CV whose header states no contact value at all, and a
  structured résumé that states their name and phone
- **THEN** the block carries the structured résumé's name and phone

#### Scenario: Sources are not merged

- **WHEN** a user's base CV header states a name but no phone, while their structured
  résumé states both
- **THEN** the block carries the base CV's name and an empty phone

#### Scenario: A tailored CV is never a source

- **WHEN** a user's only CVs are tailored copies
- **THEN** the tailored copies are ignored and the structured résumé answers instead

### Requirement: The account email backstops an unstated address

The system SHALL fall back to the account's own email address whenever the resolved
contact block states no email — including when no source answered at all. Every other
field stays empty in that case.

An account address is a verified fact about the caller that needs no CV, so a user who has
uploaded nothing still gets the one field an application form always asks for.

#### Scenario: A user with no CV and no résumé still gets their address

- **WHEN** an authenticated caller has neither a CV nor a structured résumé
- **THEN** the block carries the account email and every other field is empty

#### Scenario: A source stating no email falls back to the account address

- **WHEN** the resolved source states a name and a phone but no email
- **THEN** the block carries that name and phone together with the account email

### Requirement: Both autofill entry points share one assembly

The system SHALL assemble the contact block once and serve it to both the extension's
deterministic read (`GET /api/v1/me/autofill-profile`) and the agent-driven fill
(`POST /api/v1/me/autofill/run`), so the values a person sees in a form and the values the
agent grounds its plan in cannot diverge. Both entry points remain authenticated and
accept an API key, because the browser extension holds one.

#### Scenario: The agent fills from the same block the endpoint serves

- **WHEN** the agent-driven autofill runs for a user
- **THEN** it grounds its plan in the same contact block the read endpoint would serve that
  user

### Requirement: Autofill reads through the domain stores

The system SHALL read the base CV through the CV store and the account and structured
résumé through generated queries, and SHALL NOT issue hand-written SQL for them. A
hand-written statement is invisible to the query generator, so a column renamed in a
migration breaks autofill when a user opens a form rather than when the project is built.

#### Scenario: A renamed column fails the build

- **WHEN** a column the autofill block reads is renamed in a migration and the queries are
  regenerated
- **THEN** the project fails to build, rather than compiling and failing at request time
