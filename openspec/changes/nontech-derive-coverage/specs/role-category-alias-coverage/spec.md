## ADDED Requirements

### Requirement: Administrative assistant title variants resolve to administration

The title-alias dictionary SHALL resolve the abbreviated and coordinator/specialist
spellings of administrative support work to the `administration` category: "Admin
Assistant", "Administrative Coordinator", "Administrative Specialist", "Front Desk",
and "Virtual Assistant". The longhand "Administrative Assistant", "Executive
Assistant", "Office Manager", "Office Assistant" and "Receptionist" already resolve
and are unchanged.

#### Scenario: Abbreviated admin assistant resolves

- **WHEN** a title is "Admin Assistant" or "Senior Admin Assistant"
- **THEN** the derived category is `administration`

#### Scenario: Coordinator and specialist spellings resolve

- **WHEN** a title is "Administrative Coordinator" or "Administrative Specialist"
- **THEN** the derived category is `administration`

#### Scenario: Front desk resolves

- **WHEN** a title is "Front Desk Agent" or "Front Desk Coordinator"
- **THEN** the derived category is `administration`

#### Scenario: Virtual assistant resolves

- **WHEN** a title is "Virtual Assistant" or "Executive Virtual Assistant"
- **THEN** the derived category is `administration`

### Requirement: Immigration practice titles resolve to legal

The title-alias dictionary SHALL resolve the immigration-practice family to the
`legal` category: "Immigration Paralegal", "Immigration Specialist", "Immigration
Assistant", "Immigration Consultant", and "Immigration Case Manager".

#### Scenario: Immigration paralegal resolves

- **WHEN** a title is "Immigration Paralegal"
- **THEN** the derived category is `legal`

#### Scenario: Immigration case manager resolves to legal, not management

- **WHEN** a title is "Immigration Case Manager"
- **THEN** the derived category is `legal`
- **AND** it is not stolen by the terminal `manager` fall-through

#### Scenario: Immigration assistant resolves to legal, not administration

- **WHEN** a title is "Immigration Assistant"
- **THEN** the derived category is `legal`

### Requirement: The bare alias "assistant" is never admitted

The title-alias dictionary SHALL NOT carry a bare `assistant` entry. In live titles
the word most often states a GRADE ("Assistant Controller", "Assistant
Superintendent") or qualifies a trade that is not administrative work ("Maintenance
Assistant", "Clinic Assistant"). Only qualified assistant phrases earn an entry.

This is the same class of exclusion the dictionary already keeps for bare "safe" and
bare "compliance", and it is what makes qualifier entries and `categoryNone`
sentinels unnecessary for the grade-word tail.

#### Scenario: A grade-word assistant title stays unresolved

- **WHEN** a title is "Assistant Controller" or "Assistant Superintendent" or
  "Assistant Director"
- **THEN** the dictionary does not resolve it to `administration`

#### Scenario: A trade assistant title stays unresolved

- **WHEN** a title is "Maintenance Assistant" or "Clinic Assistant" or "Laboratory
  Assistant"
- **THEN** the dictionary does not resolve it to `administration`

#### Scenario: Qualified assistant phrases still resolve

- **WHEN** a title is "Virtual Assistant" or "Administrative Assistant"
- **THEN** the derived category is `administration`
