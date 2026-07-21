## ADDED Requirements

### Requirement: The role-category vocabulary is canonical and fully defined

The system SHALL define a single canonical role-category vocabulary in which every value has a one-line meaning and belongs to exactly one partition — **technical** (core IT/product roles) or **business** (back-office roles) — plus the residual `other`. The vocabulary SHALL be the union of the existing categories and the ten roles this change introduces, and SHALL cover the roles that exist inside a technology company beyond engineering.

The **technical** partition SHALL contain: `backend`, `frontend`, `fullstack`, `mobile`, `devops`, `sre`, `network_engineering`, `data_engineering`, `data_science`, `data_analytics`, `ml_ai`, `ai_engineering`, `qa`, `security`, `hardware`, `embedded`, `blockchain`, `architecture`, `design`, `product`, `project_management`, and the new `business_analysis`, `solutions_engineering`, `developer_relations`, `technical_writing`.

The **business** partition SHALL contain: `marketing`, `sales`, `support`, `management`, and the new `recruiting`, `hr`, `finance`, `legal`, `operations`, `customer_success`.

`other` SHALL remain the residual for a title that resolves to none of the above.

#### Scenario: Every category has a definition and a partition

- **WHEN** the canonical vocabulary is enumerated
- **THEN** each value carries a one-line definition and is a member of exactly one of the technical, business, or residual (`other`) partitions, with no value in two partitions and none absent

#### Scenario: New IT-company roles are present

- **WHEN** the vocabulary is checked for the ten introduced roles
- **THEN** `recruiting`, `hr`, `finance`, `legal`, `operations`, `customer_success`, `business_analysis`, `solutions_engineering`, `developer_relations`, and `technical_writing` are all present, each with a definition and partition

### Requirement: The partition sets exactly cover the vocabulary

The technical partition (`TechCategories`), the business/non-technical partition (`NonTechCategories`), and `{"other"}` SHALL partition the full category vocabulary (`CategoryValues`) exactly — their union SHALL equal `CategoryValues` and they SHALL be pairwise disjoint. Adding a new category SHALL require placing it in exactly one partition, enforced by an automated test.

#### Scenario: Union equals the vocabulary

- **WHEN** the three sets are unioned
- **THEN** the result equals `CategoryValues` with no duplicates and no missing value

#### Scenario: A new category placed in neither partition fails the test

- **WHEN** a category is added to `CategoryValues` but to neither `TechCategories` nor `NonTechCategories` (and it is not `other`)
- **THEN** the partition test fails, forcing an explicit tech/business placement

### Requirement: The ten new IT-company roles are derived from titles

The deterministic title-classification dictionary SHALL resolve the ten new categories from their curated English and Russian title aliases by whole-word match, and MUST NOT guess — a title it cannot confidently place yields no category. Each new category SHALL be reachable from its role titles (e.g. "Recruiter" → `recruiting`, "HR Business Partner" → `hr`, "Financial Analyst" → `finance`, "Legal Counsel" → `legal`, "Operations Manager" → `operations`, "Customer Success Manager" → `customer_success`, "Business Analyst" → `business_analysis`, "Sales Engineer" → `solutions_engineering`, "Developer Advocate" → `developer_relations`, "Technical Writer" → `technical_writing`).

#### Scenario: A new-role title resolves to its category

- **WHEN** a job title states one of the new roles (e.g. "Senior Business Analyst")
- **THEN** the derived category is the corresponding new value (`business_analysis`)

#### Scenario: BI titles route to the existing analytics category

- **WHEN** a title states "Business Intelligence Analyst" or "BI Analyst"
- **THEN** the derived category is `data_analytics`, not a new BI category

### Requirement: customer_success is distinct from support

`customer_success` SHALL be a separate category from `support`. Proactive post-sale roles — customer success, onboarding, implementation, renewals — SHALL resolve to `customer_success`; reactive helpdesk/service roles SHALL remain `support`. A title previously resolving to `support` on the strength of "customer success" SHALL now resolve to `customer_success`.

#### Scenario: Customer success no longer falls under support

- **WHEN** a title states "Customer Success Manager"
- **THEN** the derived category is `customer_success`, and a "Help Desk" / "Customer Service" title still resolves to `support`

### Requirement: Alias ordering prevents terminal fall-throughs from stealing specific roles

The title dictionary SHALL order its aliases so that a specific multi-word role always wins over a generic terminal fall-through. Every `…analyst` alias (e.g. `financial analyst`, `business analyst`, `systems analyst`, `operations analyst`, `compliance analyst`) SHALL be matched before the terminal `analyst → data_analytics`; every `…manager` alias with a function (e.g. `finance manager`, `operations manager`, `legal manager`) SHALL be matched before the terminal `manager → management`; `sales engineer` SHALL be matched before bare `sales`; `ux writer` and `content designer` SHALL be matched before `ux` and `designer`. The acronym `csm` SHALL NOT be added as an alias (it collides with Certified Scrum Master).

#### Scenario: A functional analyst is not stolen by the fall-through

- **WHEN** a title states "Financial Analyst" or "Business Analyst"
- **THEN** it resolves to `finance` / `business_analysis` respectively, not to `data_analytics` via the terminal `analyst` fall-through

#### Scenario: A functional manager is not stolen by the fall-through

- **WHEN** a title states "Operations Manager"
- **THEN** it resolves to `operations`, not to `management` via the terminal `manager` fall-through

#### Scenario: Sales engineer beats bare sales

- **WHEN** a title states "Sales Engineer"
- **THEN** it resolves to `solutions_engineering`, not to `sales`

### Requirement: New-role skills extend the curated skill dictionary

The curated skill-tag dictionary SHALL be extended with the skills of the new IT-company roles (recruiting, HR, finance, legal, operations, customer success, solutions/pre-sales, business analysis, developer relations, technical writing), remaining precision-first and curated-only: it SHALL resolve only known aliases and emit nothing for unknown terms. Skills SHALL be sourced from authoritative role references (ESCO/O*NET/BABOK and industry sources), not invented.

#### Scenario: A new-role skill resolves from a description

- **WHEN** a job description for a recruiter states "sourcing on LinkedIn Recruiter with boolean search"
- **THEN** the corresponding curated skills are tagged, while an unknown term still emits nothing
