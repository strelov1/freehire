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
document and applying a CV patch. In a general chat session it SHALL additionally
be given the mail tools, which cover the CLI's `inbox` commands: the mailbox's
label counts, the filtered listing, classification, suggestion resolution,
linking and unlinking, and recording an application from a message. It SHALL
additionally be given `present_jobs`,
the one tool whose purpose is presentation rather than retrieval or state change:
it is how a vacancy reaches the user's screen. Rendering the CV SHALL NOT be a
tool: the workspace already previews the document beside the chat, so a render
would return bytes the model cannot read and the user a copy of what is on
screen. Each tool SHALL declare a
JSON schema for its arguments and return structured data, not human-formatted
text. Moderator-only operations (job authoring, submission review) SHALL NOT be
exposed.

Where a tool and an HTTP endpoint perform the same operation, they SHALL reach it
through one service rather than two implementations. A rule enforced in a Fiber
handler is a rule keyed on the transport, and the in-process agent issues no HTTP
request — such a rule would go silently unenforced for it, which is how the
CV-tailoring contact-block guard was once lost.

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

#### Scenario: A mail tool and its endpoint cannot disagree

- **WHEN** the model classifies a message through a tool and a harness classifies one through the HTTP endpoint
- **THEN** both take the same validation, write the same columns and advance the application's stage by the same rules, because both call the same service

### Requirement: A session's preset selects its prompt and its tools

A session SHALL record a preset. The general-chat preset SHALL offer the
discovery, tracking and mail tools; the CV-tailoring preset SHALL additionally offer
the CV tools and SHALL be bound to the tailored CV and its vacancy, and SHALL NOT
offer the mail tools. The preset
SHALL select the system prompt. No other behaviour SHALL differ between presets,
so the same chat surface serves both.

A preset's prompt and its tool set SHALL be chosen from one shared decision, so an
unrecognised preset cannot resolve to one preset's prompt and another's tools. Each
prompt SHALL name only tools its own preset registers: a prompt that names a tool the
session does not carry teaches the model to spend a round on a call that can only come
back unknown. The reverse SHALL NOT be required — a tool may go unnamed by the prompt,
because its own description is what the model reads to decide whether to call it.

#### Scenario: A chat session has no CV tools

- **WHEN** a general chat session runs a turn
- **THEN** no CV-editing tool is offered to the model

#### Scenario: A tailoring session is bound to its CV

- **WHEN** a tailoring session runs a turn
- **THEN** the CV tools are offered and operate on the CV the session is bound to, and the tailoring context for its vacancy is available to the model

#### Scenario: An unrecognised preset resolves consistently

- **WHEN** a session records a preset the system does not recognise
- **THEN** it runs under the general-chat prompt and the general-chat tool set — never one preset's prompt with another's tools

#### Scenario: Every tool a prompt names is registered

- **WHEN** a preset's system prompt names a tool
- **THEN** that tool is registered for that preset
