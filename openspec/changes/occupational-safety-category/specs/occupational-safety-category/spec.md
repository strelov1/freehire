## ADDED Requirements

### Requirement: The catalogue carries an occupational-safety category

The system SHALL carry `occupational_safety` as a category facet value, labelled
`Health & Safety (HSE)` on every surface that renders a category, and grouped under
`Quality & Security` in the category picker.

The key follows the two national occupational classifications that give this profession
its own unit — SOC minor group `19-5000 Occupational Health and Safety Specialists and
Technicians` and ISCO-08 unit group `2263 Environmental and occupational health and
hygiene professionals`. Neither files it under engineering, which is why it is a sibling
of `industrial_engineering` rather than a slice of it: that category is the seat a
factory or plant staffs for manufacturing, process and maintenance work, and an HSE
Manager is a compliance and controls role.

The label carries the acronym because that is what the population calls itself, and it
is the word a subscriber types.

The value SHALL be a member of both `NonTechCategories` and `NonTechCraftCategories`.
Craft membership is not decorative: the prune business rule deletes non-technical
categories at a company with no technical history and SUBTRACTS that set, so without
membership the retirement of one oil & gas or construction board would remove that
employer's entire catalogue.

#### Scenario: The category is a facet value on every surface

- **WHEN** the category vocabulary is rendered in the web filter, on a job card, or in
  the browser extension
- **THEN** `occupational_safety` appears as `Health & Safety (HSE)`, and in the web
  filter it appears inside the `Quality & Security` group

#### Scenario: The category is non-technical craft

- **WHEN** the vocabulary's category sets are evaluated
- **THEN** `occupational_safety` is a member of `NonTechCategories` and of
  `NonTechCraftCategories`, and is not a member of `TechCategories`

#### Scenario: A resolved posting derives is_tech false

- **WHEN** a posting resolves `occupational_safety` and its title carries no technical
  evidence
- **THEN** `is_tech` derives `false`, so the posting is surfaced under its facet and
  never consumes LLM enrichment budget

### Requirement: HSE title spellings resolve the category

The system SHALL resolve a title to `occupational_safety` when it carries any of the
profession's spellings as a whole word: the acronyms `HSE`, `EHS`, `HSSE`, `QHSE`,
`HSEQ`, `SHEQ`, `SHES`, `HSSE&SP` and `HSE&S`; the spelled-out forms `health and
safety`, `health & safety`, `environmental health and safety`, `environmental health &
safety`, `occupational health and safety`, and the Russian `охрана труда` / `охране
труда`; and the reported titles SOC lists for `19-5011` — `safety officer`, `safety
specialist`, `safety coordinator`, `safety advisor`, `safety supervisor`, `safety
technician`, `safety director`, `industrial hygienist` and `risk control consultant`.

`safety engineer` SHALL resolve `occupational_safety` rather than the
`industrial_engineering` it resolves today.

#### Scenario: Acronym spellings resolve

- **WHEN** a job titled "EHS Specialist", "HSE Officer", "HSEQ Advisor", "QHSE
  Coordinator" or "HSSE Manager" is classified
- **THEN** its category is `occupational_safety`

#### Scenario: Spelled-out and Russian forms resolve

- **WHEN** a job titled "Environmental Health and Safety Specialist", "Health & Safety
  Advisor" or "Инженер по охране труда" is classified
- **THEN** its category is `occupational_safety`

#### Scenario: The SOC reported titles resolve

- **WHEN** a job titled "Safety Coordinator", "Safety Specialist", "Industrial
  Hygienist" or "Safety Engineer" is classified
- **THEN** its category is `occupational_safety`

### Requirement: The HSE block outranks the bare manager fall-through

The category title table's order is its precedence, and its bare `manager` entry is the
documented fall-through for a manager title with no recognised function. The HSE alias
block SHALL be declared BEFORE that entry, so an HSE-prefixed manager title resolves the
profession rather than falling through to `management`.

This diverges from SOC, which codes EHS Managers separately from `19-5011`. The
divergence is deliberate: this system's `category` facet names the craft and `seniority`
is a separate facet, so splitting one profession across two category values would force
a subscriber to tick two boxes to see one job market.

#### Scenario: An HSE manager title resolves the profession

- **WHEN** a job titled "EHS Manager", "HSE Manager", "Regional EHS Manager" or "Senior
  EHS Manager" is classified
- **THEN** its category is `occupational_safety`, not `management`

#### Scenario: An unrelated manager title still falls through

- **WHEN** a job titled "Business Manager" — carrying no recognised function — is
  classified
- **THEN** its category is still `management`, unchanged by this block

### Requirement: Lookalike words MUST NOT resolve the category

The dictionary MUST NOT carry bare `she`, bare `risk` or bare `safety` as aliases, and a
title carrying only one of those words MUST NOT resolve `occupational_safety`.

`she` is an ordinary English word and live titles carry pronouns, so only the qualified
forms (`she manager`, `she officer`, `she advisor`, `she coordinator`, `she specialist`)
are admitted. `risk` matched 41,468 live postings, mostly finance, so only `hse risk`
and `safety risk` are admitted alongside SOC's `risk control consultant`. Bare `safety`
names other professions in live titles — patient safety in healthcare, campus and public
safety in protective services, food safety in manufacturing quality — and the qualified
title forms cover the population without it.

This follows the precedent the table already sets for bare `security` and bare `mobile`.

#### Scenario: A pronoun in a title does not resolve the category

- **WHEN** a job titled "Software Engineer (she/her)" is classified
- **THEN** its category is not `occupational_safety`

#### Scenario: A finance risk title does not resolve the category

- **WHEN** a job titled "Risk Analyst" or "Credit Risk Manager" is classified
- **THEN** its category is not `occupational_safety`

#### Scenario: Other professions carrying the word safety do not resolve the category

- **WHEN** a job titled "Patient Safety Attendant" or "Public Safety Officer" is
  classified
- **THEN** its category is not `occupational_safety`

#### Scenario: A qualified SHE spelling does resolve

- **WHEN** a job titled "SHE Manager" or "SHE Advisor" is classified
- **THEN** its category is `occupational_safety`
