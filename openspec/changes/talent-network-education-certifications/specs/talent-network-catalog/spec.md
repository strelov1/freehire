## MODIFIED Requirements

### Requirement: A public response carries no free text from a CV

Every string a public catalogue response emits SHALL be one of: a term resolved by a
project dictionary, a formatted date, or a fixed label owned by the application. Numbers
are emitted as numbers. No value copied from a CV's free-text fields SHALL reach a public
response.

The public projection SHALL therefore withhold: the candidate's name, photo, email, phone,
links, free-text location, headline, summary; every experience entry's employer name,
location, summary and highlights; projects entirely; every education entry's institution
name and field of study; every certification's issuer and date; and — because no
dictionary reachable from this block resolves them — languages.

The public projection SHALL carry: total years of experience, dictionary-resolved skills,
per role the seniority and category its title resolves to, the period, and the
dictionary-resolved stack, plus — where a dictionary resolves them — each education
entry's degree level and year, and dictionary-resolved certifications.

Withholding *every* employer name, not only the current one, is the point. A candidate's
stated fear is their current employer noticing, and a work history that names the previous
three employers alongside a city and a seniority identifies a person as surely as a name
does.

#### Scenario: A CV field that names the employer

- **WHEN** a member's CV names their employer in exactly one place — the `company`
  column, the CV summary, a role's summary or highlights, the job title, the headline, a
  project's name or highlights, the institution, the free-text location, a language entry,
  or a skill token
- **THEN** that name appears nowhere in the marshalled card, for every one of those
  places taken separately

#### Scenario: A CV whose prose names the employer

- **WHEN** a member's CV summary reads "at <employer> I rebuilt the billing pipeline" and
  their current role's `company` field is also set
- **THEN** neither the summary nor the employer name appears anywhere in the catalogue
  response or in that member's card

#### Scenario: A job title that carries the employer

- **WHEN** a member's most recent role title reads "Backend Engineer @ <employer>"
- **THEN** the response carries the seniority and category that title resolves to, and not
  the title's own text

#### Scenario: A title no dictionary resolves

- **WHEN** a role's title resolves to neither a category nor a seniority
- **THEN** the role still appears, carrying its period and stack under a neutral label
- **AND** the role is not dropped from the work history

#### Scenario: A skill outside the dictionary

- **WHEN** a member's CV lists a skill the skill dictionary does not resolve
- **THEN** that skill is absent from the response, and the resolved ones are present

#### Scenario: An education entry whose degree resolves

- **WHEN** a member's CV lists an education entry whose degree text resolves to a level in
  the closed education-level vocabulary
- **THEN** the response carries that level and the entry's year
- **AND** the entry's institution name and field of study appear nowhere in the response

#### Scenario: An education entry whose degree does not resolve

- **WHEN** a member's CV lists an education entry whose degree text resolves to no level
  in the closed vocabulary
- **THEN** that entry is absent from the response entirely, and any of the member's other
  education entries that do resolve are still present

#### Scenario: A certification the dictionary resolves

- **WHEN** a member's CV lists a certification whose name the certification dictionary
  resolves to a canonical entry
- **THEN** the response carries that canonical certification
- **AND** no issuer or date for it appears anywhere in the response, since the underlying
  CV data carries neither

#### Scenario: A certification outside the dictionary

- **WHEN** a member's CV lists a certification the certification dictionary does not
  resolve
- **THEN** that certification is absent from the response, and the resolved ones are
  present
