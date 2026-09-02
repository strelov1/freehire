## ADDED Requirements

### Requirement: Industrial engineering is a category of its own

The system SHALL resolve the engineering seats a factory, plant, utility or
field-service organisation staffs — manufacturing, process, quality,
maintenance, controls, commissioning, reliability and field service — to an
`industrial_engineering` category.

It MUST be a member of the non-technical category set: an IT job board is not
where a process engineer looks for work, so the postings are filterable but
consume no LLM enrichment or embedding budget. That is the same placement
`engineering_design` carries, and for the same reason.

#### Scenario: The industrial engineering seats resolve

- **WHEN** a job titled "Project Engineer", "Quality Engineer", "Process
  Engineer", "Manufacturing Engineer", "Maintenance Engineer", "Controls
  Engineer", "Automation Engineer", "Field Service Engineer", "Reliability
  Engineer", "Commissioning Engineer" or "Industrial Engineer" is classified
- **THEN** its category is `industrial_engineering`

#### Scenario: The category is non-technical

- **WHEN** a job resolves to `industrial_engineering`
- **THEN** its derived `is_tech` is `false`, and it is not enqueued for AI
  enrichment or semantic embedding

### Requirement: A non-technical craft category is never deletable as business work

`cmd/prune`'s business rule deletes the non-technical categories at a company
with no technical history. That set means back-office and go-to-market work —
it must NOT reach a category that is non-technical because the CRAFT is outside
IT, since deleting those would take out an engineering employer's whole
catalogue the moment its board is retired.

The system SHALL express this as a named set of non-technical craft categories
that the business rule subtracts, and the set MUST be a subset of the
non-technical categories, enforced by a test. A single category named inline
cannot express a set of two, and the next craft category added would silently
become deletable.

#### Scenario: Both craft categories are spared

- **WHEN** the prune business rule evaluates a posting whose category is
  `engineering_design` or `industrial_engineering`
- **THEN** the rule does not treat it as business work, and the posting is not
  deleted

#### Scenario: Back-office categories are still deletable

- **WHEN** the same rule evaluates a posting whose category is `marketing`,
  `sales`, `recruiting` or `finance`
- **THEN** the rule treats it as business work, exactly as before

#### Scenario: The craft set cannot drift from the vocabulary

- **WHEN** the vocabulary is checked
- **THEN** every member of the craft set is also a member of the non-technical
  set

### Requirement: The Russian engineering vocabulary resolves

Roughly half the gap is Russian, and none of it carries an English alias. The
system SHALL resolve `инженер` and the qualified forms the catalogue carries.
The bare token is required: Russian qualified forms hyphenate
(`Инженер-технолог`, `Инженер-электроник`) or postfix a prepositional phrase
(`Инженер по наладке и испытаниям`), and a hyphen is a word boundary, so only
the bare token reaches every spelling.

Qualified forms that name a DIFFERENT discipline MUST be declared above the
bare token and routed to it, not left to fall through.

#### Scenario: The Russian engineering titles resolve

- **WHEN** a job titled "Инженер", "Инженер-технолог", "Инженер-энергетик",
  "Инженер ПТО", "Инженер по подготовке производства", "Инженер-механик" or
  "Главный инженер" is classified
- **THEN** its category is `industrial_engineering`

#### Scenario: A Russian title naming another discipline keeps it

- **WHEN** a job titled "Инженер-проектировщик" or "Инженер по защите
  информации" is classified
- **THEN** its category is `engineering_design` and `security` respectively —
  the first is a draughtsman, the second an information-security engineer, and
  the bare `инженер` alias declared below them must not claim either

### Requirement: The bare engineer alias never claims a discipline that names itself

`Engineer` (689 open) and `Инженер` (1 543) are the largest bare spellings left,
and every software, data, security and infrastructure discipline already names
itself in its own title. The system SHALL declare the bare aliases LAST, below
every discipline-specific alias, and SHALL declare above them the IT titles that
would otherwise fall through: `IT Engineer`, `Database Engineer`, `Business
Intelligence Engineer` and `Electronics Engineer`.

#### Scenario: The IT lookalikes keep their own discipline

- **WHEN** a job titled "IT Engineer", "Database Engineer", "Business
  Intelligence Engineer" or "Electronics Engineer" is classified
- **THEN** its category is `software_engineering`, `devops`, `data_analytics`
  and `hardware` respectively, not `industrial_engineering`

#### Scenario: The software disciplines are untouched

- **WHEN** a job titled "Backend Engineer", "Data Engineer", "Security
  Engineer", "Site Reliability Engineer", "Sales Engineer" or "Systems
  Engineer" is classified
- **THEN** its category is unchanged by this vocabulary

#### Scenario: A bare engineer title resolves to the industrial category

- **WHEN** a job titled "Engineer", "Engineer II" or "Associate Engineer" is
  classified, naming no discipline of its own
- **THEN** its category is `industrial_engineering`

### Requirement: The industrial crafts expose named roles

The bare category role says "some engineering seat outside software". The
system SHALL name the crafts so a candidate filters by the seat:
`project_engineer`, `quality_engineer`, `process_engineer`,
`maintenance_engineer`, `controls_engineer`, `automation_engineer`,
`field_service_engineer` and `industrial_engineer`.

#### Scenario: An industrial craft resolves to its own role

- **WHEN** a job titled "Senior Project Engineer" or "Field Service Engineer"
  is classified
- **THEN** its roles include `project_engineer` and `field_service_engineer`
  respectively

#### Scenario: The named roles do not steal from the design crafts

- **WHEN** a job titled "Mechanical Design Engineer" or "Design Engineer" is
  classified
- **THEN** its roles are unchanged — `mechanical_designer` and
  `design_engineer` respectively

### Requirement: The category is labelled and selectable

The system SHALL label `industrial_engineering` on every surface that renders a
facet code — the generated web contracts, the web picker's section map, and the
extension's own exhaustive label map — and SHALL give it a role noun, without
which the seniority×category role grid cannot name it.

#### Scenario: The category renders and can be chosen

- **WHEN** a user opens the category filter
- **THEN** `industrial_engineering` appears with a human label in the craft
  section, and selecting it filters the job list

#### Scenario: The graded roles are nameable

- **WHEN** the role catalogue is built
- **THEN** `industrial_engineering` has a role noun, so the bare and graded
  roles (`senior_industrial_engineering`) carry a label
