## MODIFIED Requirements

### Requirement: The tailoring agent never receives the CV contact block

The CV contact block MUST NOT reach any reader that puts the document in front of a model. That
covers both agent-facing read paths — the HTTP read authenticated with the short-lived tailoring
key, and the in-process assistant's `cv_get` tool — each of which SHALL omit the `Header` contact
fields (`full_name`, `email`, `phone`, and the personal links list) from the document it returns,
and SHALL reject any patch that targets those fields.

The omission SHALL be performed by one shared redaction in the service path, applied by every
agent-facing reader, rather than by a per-transport check in a handler. A guard keyed on how the
request arrived (holding an API key, say) does not survive a new agent surface that arrives over a
different transport or none at all, and the requirement is about who is reading, not how they got
there. A new surface must not be able to inherit the CV-reading capability without inheriting the
redaction.

The stored contact values are unchanged and appear in the rendered output (served on the owner's
own cookie-authenticated read and the PDF), so the finished CV is complete while the agent's model
never sees the identifiers. The candidate's own cookie-authenticated reads are unaffected.

#### Scenario: Agent read omits the contact block

- **WHEN** the tailoring key is used to read the CV document
- **THEN** the response document carries the body (experience, summary, skills, …) but no `full_name`, `email`, or `phone`

#### Scenario: The in-process tool read omits the contact block

- **WHEN** the assistant's `cv_get` tool reads the CV document during a tailoring session, holding no API key
- **THEN** the tool result carries the body but no `full_name`, `email`, `phone`, or personal links

#### Scenario: Agent cannot patch a contact field

- **WHEN** the tailoring key is used to patch `full_name`, `email`, or `phone`
- **THEN** the patch is rejected and the stored contact value is unchanged

#### Scenario: The owner still sees and renders full contacts

- **WHEN** the owner reads the CV over their cookie session, or the CV is rendered to PDF
- **THEN** the real contact block is present

#### Scenario: The tailored body carries no contact identifier back

- **WHEN** the agent patches the CV body during tailoring
- **THEN** no contact identifier is introduced into a body field (the agent never held one)
