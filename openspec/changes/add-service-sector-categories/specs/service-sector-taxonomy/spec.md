## ADDED Requirements

### Requirement: Four service sectors become categories

The system SHALL resolve the remaining coherent service work to four
categories: `logistics`, `education`, `personal_services` and
`administration`. All four MUST be members of the non-technical category set,
and MUST NOT be members of the craft set `cmd/prune` subtracts — the same call
the consumer industries carry, and for the same reason: they are the business
that rule exists to remove, and leaving them out is what keeps categorising
them behaviour-neutral.

#### Scenario: Logistics titles resolve

- **WHEN** a job titled "Delivery Specialist", "Commercial Driver", "Driver",
  "Warehouse Associate", "Forklift Operator", "Dispatcher", "Courier" or
  "Fulfillment Associate" is classified
- **THEN** its category is `logistics`

#### Scenario: Education titles resolve

- **WHEN** a job titled "Swim Instructor", "Teacher", "Tutor", "Preschool
  Teacher", "Soccer Coach" or "Lecturer" is classified
- **THEN** its category is `education`

#### Scenario: Personal-service titles resolve

- **WHEN** a job titled "Stylist", "Barber", "Esthetician", "Lifeguard",
  "Security Guard", "Janitor" or "Housekeeper" is classified
- **THEN** its category is `personal_services`

#### Scenario: An ambiguous guard title is not claimed

- **WHEN** a job titled "Security Officer" or "Chief Information Security
  Officer" is classified
- **THEN** its category is `security`, and the multi-category CV path does NOT
  additionally report `personal_services` — `Categories` returns every matching
  alias rather than the strongest, so declaration order cannot save it and the
  phrase is dropped rather than guessed

#### Scenario: Administration titles resolve

- **WHEN** a job titled "Receptionist", "Secretary", "Office Manager",
  "Administrative Assistant" or "Data Entry Specialist" is classified
- **THEN** its category is `administration`

#### Scenario: All four are non-technical and not craft-protected

- **WHEN** the vocabulary is checked
- **THEN** all four are members of the non-technical set and none is a member
  of the craft set

### Requirement: The Russian service vocabularies resolve

The system SHALL resolve the Russian service titles, which carry no English
alias: the logistics family (`водитель`, `курьер`, `кладовщик`, `экспедитор`,
`грузчик`, `сборщик заказов`), the education family (`педагог`,
`воспитатель`, `преподаватель`, `методист`), administration (`секретарь`,
`делопроизводитель`) and personal services (`парикмахер`, `уборщик`,
`охранник`, `сиделка`).

#### Scenario: The Russian families resolve

- **WHEN** a job titled "Водитель", "Кладовщик", "Сборщик заказов",
  "Педагог-психолог", "Помощник воспитателя", "Секретарь руководителя" or
  "Парикмахер" is classified
- **THEN** its category is `logistics`, `logistics`, `logistics`, `education`,
  `education`, `administration` and `personal_services` respectively

### Requirement: A bare alias never claims the word inside a longer one

`Делопроизводитель` — an office clerk — ends in `водитель`, "driver", and this
change adds `водитель` as a bare alias for the first time. The system relies on
whole-word matching to keep them apart and MUST carry a regression test for the
pair, because nothing in the alias list shows the hazard.

The two MUST additionally resolve to DIFFERENT categories, so a test that only
asserted "resolves to something" would pass while the rows were wrong.

#### Scenario: The clerk is not a driver

- **WHEN** a job titled "Делопроизводитель" is classified
- **THEN** its category is `administration`, not `logistics`

### Requirement: Building-maintenance work joins the trades

The Russian general-maintenance titles the catalogue carries in volume —
`Рабочий по комплексному обслуживанию и ремонту зданий` (4 120) and `Рабочий по
благоустройству населенных пунктов` (1 136) — name the same work the trades
vocabulary already covers. The system SHALL resolve them to `skilled_trades`.

#### Scenario: The maintenance workers resolve to the trades

- **WHEN** a job titled "Рабочий по комплексному обслуживанию и ремонту
  зданий" or "Рабочий по благоустройству населенных пунктов" is classified
- **THEN** its category is `skilled_trades`

### Requirement: The service seats expose named roles

The system SHALL name the seats the catalogue carries in volume: `driver`,
`warehouse_associate`, `dispatcher`, `teacher`, `instructor`, `receptionist`,
`stylist`, `security_guard` and `cleaner`.

#### Scenario: A service seat resolves to its own role

- **WHEN** a job titled "Commercial Driver", "Swim Instructor" or "Master
  Stylist" is classified
- **THEN** its roles include `driver`, `instructor` and `stylist` respectively

### Requirement: The four categories are labelled and selectable

The system SHALL label all four on every surface that renders a facet code and
place them in the web picker's consumer-services group, and SHALL give each a
role noun without which the bare and graded roles cannot be named.

#### Scenario: The categories render and can be chosen

- **WHEN** a user opens the category filter
- **THEN** all four appear with human labels under the consumer-services
  group, and selecting one filters the job list
