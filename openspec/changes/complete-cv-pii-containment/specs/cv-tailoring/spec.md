## ADDED Requirements

### Requirement: The tailoring agent never receives the CV contact block

Because the tailoring agent runs its own model over the CV, the CV contact block MUST NOT reach
it. When a CV is read or patched with the short-lived tailoring key, the system SHALL omit the
`Header` contact fields (`full_name`, `email`, `phone`) from the returned document and SHALL
reject any patch that targets those fields. The stored contact values are unchanged and appear in
the rendered output (served on the owner's own cookie-authenticated read and the PDF), so the
finished CV is complete while the agent's model never sees the identifiers. The candidate's own
cookie-authenticated reads are unaffected.

#### Scenario: Agent read omits the contact block

- **WHEN** the tailoring key is used to read the CV document
- **THEN** the response document carries the body (experience, summary, skills, …) but no `full_name`, `email`, or `phone`

#### Scenario: Agent cannot patch a contact field

- **WHEN** the tailoring key is used to patch `full_name`, `email`, or `phone`
- **THEN** the patch is rejected and the stored contact value is unchanged

#### Scenario: The owner still sees and renders full contacts

- **WHEN** the owner reads the CV over their cookie session, or the CV is rendered to PDF
- **THEN** the real contact block is present

#### Scenario: The tailored body carries no contact identifier back

- **WHEN** the agent patches the CV body during tailoring
- **THEN** no contact identifier is introduced into a body field (the agent never held one)
