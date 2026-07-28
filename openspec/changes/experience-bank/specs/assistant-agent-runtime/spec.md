## MODIFIED Requirements

### Requirement: A session's preset selects its prompt and its tools

A session SHALL record a preset. The general-chat preset SHALL offer the
discovery and tracking tools; the CV-tailoring preset SHALL additionally offer
the CV tools and SHALL be bound to the tailored CV and its vacancy; the
profile preset SHALL offer no additional tools and SHALL run the
experience-interviewer prompt. The **experience-bank tools SHALL be registered
under every preset**, because the moment a candidate articulates their experience
is not confined to one surface. The preset SHALL select the system prompt. No
other behaviour SHALL differ between presets, so the same chat surface serves all
of them.

#### Scenario: A chat session has no CV tools

- **WHEN** a general chat session runs a turn
- **THEN** no CV-editing tool is offered to the model

#### Scenario: A tailoring session is bound to its CV

- **WHEN** a tailoring session runs a turn
- **THEN** the CV tools are offered and operate on the CV the session is bound to, and the tailoring context for its vacancy is available to the model

#### Scenario: Every preset can record experience

- **WHEN** a session runs a turn under any preset
- **THEN** the experience-bank tools are offered to the model

#### Scenario: The profile preset differs only by prompt

- **WHEN** a profile session runs a turn
- **THEN** it runs the experience-interviewer prompt and is offered the same tool set as a general chat session plus the experience-bank tools
