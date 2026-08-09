## ADDED Requirements

### Requirement: Forward Deployed Engineer resolves from FDE and its synonym titles

The `forward_deployed_engineer` named role SHALL resolve from the bare
whole-word title token `FDE` and from any title containing the phrase
`forward deploy` (covering `Forward Deployed Engineer`, `Forward Deploy
Engineer`, and the hyphenated `Forward-Deployed Engineer` spelling), matching
the same case-insensitive rule the field guide's own FDE analysis uses.

The system SHALL additionally provide two separate named roles for the
synonym titles the field guide documents as the same class of work under a
different name — `applied_ai_engineer` (alias: "applied ai engineer") and
`deployment_engineer` (alias: "deployment engineer") — kept as their own
slugs rather than merged into `forward_deployed_engineer`, so a job's derived
role reflects the title it was actually posted under.

#### Scenario: Bare FDE title resolves

- **WHEN** `roletag` derives roles for a job titled "FDE - Enterprise AI"
- **THEN** the derived `roles` include `forward_deployed_engineer`

#### Scenario: Hyphenated forward-deployed spelling resolves

- **WHEN** `roletag` derives roles for a job titled "Forward-Deployed Engineer"
- **THEN** the derived `roles` include `forward_deployed_engineer`

#### Scenario: Applied AI Engineer resolves to its own role, not FDE

- **WHEN** `roletag` derives roles for a job titled "Applied AI Engineer"
- **THEN** the derived `roles` include `applied_ai_engineer` and do NOT
  include `forward_deployed_engineer`

#### Scenario: Deployment Engineer resolves to its own role

- **WHEN** `roletag` derives roles for a job titled "Deployment Engineer"
- **THEN** the derived `roles` include `deployment_engineer`
