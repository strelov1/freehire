## ADDED Requirements

### Requirement: Autopilot aligns surfaces before the unattended turn

When an autopilot run starts, the system SHALL apply the jd-surface-align rewrite
to the session's bound tailored CV against the vacancy description before the
unattended turn's first model call. A document that is already aligned SHALL be
left unchanged (idempotent). The rewrite SHALL be committed through the CV editor
as its own revision, not as a silent overwrite and not as part of the run's later
edit batch, and SHALL NOT consume agent tool rounds or require evidence citations
for chip and unambiguous-prose replacements.

#### Scenario: Autopilot start rewrites before the model runs

- **WHEN** an autopilot run starts on a tailored CV that still says `IaC` while the
  vacancy prefers `infrastructure as code`
- **THEN** the stored document is updated to the vacancy form before the turn's
  first model call

#### Scenario: Already-aligned documents are a no-op

- **WHEN** an autopilot run starts on a document whose surfaces already match the
  vacancy preferences
- **THEN** the document is unchanged and the turn proceeds

#### Scenario: Undoing the run does not revert alignment

- **WHEN** an autopilot run that performed surface alignment is undone
- **THEN** the JD-aligned wording remains and the run's other edits are reverted

### Requirement: The unattended brief states surfaces are already aligned

The server-owned autopilot brief SHALL tell the model that skill surface forms
already match the vacancy and that it MUST NOT rename skills for wording.

#### Scenario: Brief forbids synonym hunting

- **WHEN** an autopilot turn starts
- **THEN** the brief includes an instruction that wording alignment is done and
  must not be repeated
