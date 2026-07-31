## MODIFIED Requirements

### Requirement: A session's preset selects its prompt and its tools

A session SHALL record a preset. The general-chat preset SHALL offer the
discovery, tracking and mail tools; the CV-tailoring preset SHALL additionally offer
the CV tools and SHALL be bound to the tailored CV and its vacancy, and SHALL NOT
offer the mail tools; the interview-rehearsal preset SHALL be bound to a vacancy
alone, SHALL additionally offer the rehearsal context tool for that vacancy, and SHALL
offer neither the CV tools nor the mail tools nor the page tool. The preset
SHALL select the system prompt. No other behaviour SHALL differ between presets,
so the same chat surface serves both.

A preset's prompt and its tool set SHALL be chosen from one shared decision, so an
unrecognised preset cannot resolve to one preset's prompt and another's tools. Each
prompt SHALL name only tools its own preset registers: a prompt that names a tool the
session does not carry teaches the model to spend a round on a call that can only come
back unknown. The reverse SHALL NOT be required — a tool may go unnamed by the prompt,
because its own description is what the model reads to decide whether to call it.

The preset vocabulary SHALL be pinned in the database, so a session can never record a
preset the application does not implement.

#### Scenario: A chat session has no CV tools

- **WHEN** a general chat session runs a turn
- **THEN** no CV-editing tool is offered to the model

#### Scenario: A tailoring session is bound to its CV

- **WHEN** a tailoring session runs a turn
- **THEN** the CV tools are offered and operate on the CV the session is bound to, and the tailoring context for its vacancy is available to the model

#### Scenario: A rehearsal session edits no CV and reads no mail

- **WHEN** an interview-rehearsal session runs a turn
- **THEN** the rehearsal context for its vacancy is offered, and no CV-editing tool, no mail tool and no page tool is offered

#### Scenario: An unrecognised preset resolves consistently

- **WHEN** a session records a preset the system does not recognise
- **THEN** it runs under the general-chat prompt and the general-chat tool set — never one preset's prompt with another's tools

#### Scenario: Every tool a prompt names is registered

- **WHEN** a preset's system prompt names a tool
- **THEN** that tool is registered for that preset
