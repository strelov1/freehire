## ADDED Requirements

### Requirement: Agile/PM sub-role titles resolve to project_management

The title-alias dictionary SHALL resolve "Agile Coach", "Release Train Engineer", "Agile
Transformation Lead"/"Agile Transformation Manager", "Scaled Agile Framework", "SAFe
Practitioner", and "SAFe Scrum Master" to the `project_management` category, matching as
phrases (not the bare word "safe" or "agile" alone).

#### Scenario: Agile Coach resolves to project_management
- **WHEN** a title is "Agile Coach" or "Senior Agile Coach"
- **THEN** the derived category is `project_management`

#### Scenario: Release Train Engineer resolves to project_management
- **WHEN** a title is "Release Train Engineer" or "RTE / Release Train Engineer"
- **THEN** the derived category is `project_management`

#### Scenario: SAFe-qualified titles resolve to project_management
- **WHEN** a title is "SAFe Scrum Master" or "SAFe Practitioner" or contains "Scaled Agile
  Framework"
- **THEN** the derived category is `project_management`

#### Scenario: Bare "safe" does not resolve on its own
- **WHEN** a title contains the standalone word "Safe" without an agile-qualifying phrase
  (e.g. "Safe Driving Instructor")
- **THEN** the dictionary does not resolve it to `project_management` via this entry

### Requirement: Security sub-role titles resolve to security

The title-alias dictionary SHALL resolve the following phrases to the `security` category:
"IAM"/"Identity and Access Management", "GRC"/"Governance, Risk and Compliance",
"Vulnerability Management"/"Vulnerability Analyst", "Incident Response", "Red Team"/"Red
Teamer", "Blue Team", "Penetration Tester"/"Pentest", "Threat Intelligence"/"Threat Intel",
"CISO"/"Chief Information Security Officer", and "DevSecOps". Bare "compliance" SHALL NOT
be added as a `security` alias.

#### Scenario: IAM title resolves to security
- **WHEN** a title is "IAM Engineer" or "Identity and Access Management Analyst"
- **THEN** the derived category is `security`

#### Scenario: DevSecOps resolves to security, not devops
- **WHEN** a title is "DevSecOps Engineer"
- **THEN** the derived category is `security`

#### Scenario: Red/Blue team titles resolve to security
- **WHEN** a title is "Red Team Operator" or "Blue Team Analyst"
- **THEN** the derived category is `security`

#### Scenario: Bare "compliance" is not routed to security
- **WHEN** a title contains only the bare word "Compliance" with no security-qualifying
  phrase (e.g. "Customs Compliance Specialist")
- **THEN** the dictionary does not resolve it to `security` via this entry

### Requirement: Data/ML/DevOps sub-role titles resolve to their existing category

The title-alias dictionary SHALL resolve "Data Platform", "Data Governance", "Data
Steward", and "MLOps"/"ML Ops" to `data_engineering` (MLOps to `devops`, not
`data_engineering` — see below), "Analytics Engineer" to `data_analytics`, and "Platform
Engineering" (the gerund/discipline form, alongside the pre-existing "Platform Engineer"
role-noun form) to `devops`.

#### Scenario: Data platform/governance/steward titles resolve to data_engineering
- **WHEN** a title is "Data Platform Engineer", "Data Governance Lead", or "Data Steward"
- **THEN** the derived category is `data_engineering`

#### Scenario: MLOps resolves to devops
- **WHEN** a title is "MLOps Engineer" or "ML Ops Engineer"
- **THEN** the derived category is `devops`

#### Scenario: Analytics Engineer resolves to data_analytics
- **WHEN** a title is "Analytics Engineer" or "Senior Analytics Engineer"
- **THEN** the derived category is `data_analytics`

#### Scenario: Platform Engineering discipline form resolves to devops
- **WHEN** a title is "Platform Engineering Team Leader" or "Senior Platform Engineering
  Lead", which the pre-existing "platform engineer" alias does not match
- **THEN** the derived category is `devops`

### Requirement: New aliases never steal from a more specific existing alias

Every alias added for this change SHALL be ordered above the generic terminal
fall-throughs it could otherwise be caught by (e.g. bare `{"analyst", "data_analytics"}`,
bare `{"manager", "management"}`), and SHALL NOT be reordered relative to any other
existing specific alias.

#### Scenario: Vulnerability Analyst is not stolen by the bare analyst fall-through
- **WHEN** a title is "Vulnerability Analyst"
- **THEN** the derived category is `security`, not `data_analytics`

#### Scenario: Agile Transformation Manager is not stolen by the bare manager fall-through
- **WHEN** a title is "Agile Transformation Manager"
- **THEN** the derived category is `project_management`, not `management`

#### Scenario: Data Governance Manager is not stolen by the bare manager fall-through
- **WHEN** a title is "Data Governance Manager"
- **THEN** the derived category is `data_engineering`, not `management`
