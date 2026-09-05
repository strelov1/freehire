## ADDED Requirements

### Requirement: The wizard pre-fills years of experience and profile links from the résumé

The system SHALL pre-fill the wizard's years-of-experience step and its profile-links step
from the uploaded résumé's extraction, which already yields a total-years figure and a list
of links, so the candidate confirms a fact already on their CV instead of retyping it.

A link is offered against a named field by recognising its destination — a LinkedIn member
URL fills the LinkedIn field, a GitHub URL fills the GitHub field — rather than by asking the
candidate to sort the extracted list themselves. Links the extraction found but could not
recognise MUST be preserved, not dropped.

A pre-filled value is a suggestion, not an answer: it MUST remain editable and clearable, and
it MUST be recorded as the candidate's own only once they pass through the step that offers
it. An extraction that yields no total-years figure and no links MUST leave both steps empty
rather than blocking or skipping them.

#### Scenario: Total years is offered for confirmation

- **WHEN** a user uploads a résumé from which a total-years figure is extracted
- **THEN** the years-of-experience step opens with that figure pre-filled and editable

#### Scenario: Extracted links land in their named fields

- **WHEN** an extraction yields a LinkedIn member URL and a GitHub URL
- **THEN** the LinkedIn field is pre-filled with the former and the GitHub field with the latter

#### Scenario: Unrecognised links survive

- **WHEN** an extraction yields a personal site URL alongside a LinkedIn URL
- **THEN** the LinkedIn field is pre-filled and the personal site URL is still retained

#### Scenario: An extraction yielding neither leaves the steps empty

- **WHEN** an extraction yields no total-years figure and no links
- **THEN** both steps are presented empty and remain answerable by hand

#### Scenario: A correction replaces the suggestion

- **WHEN** a user edits a pre-filled years figure before leaving the step
- **THEN** their edited value is stored, and a later résumé re-upload does not overwrite it
