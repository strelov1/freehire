# consumer-industry-taxonomy Specification

## Purpose

The consumer industries a broad multi-industry ATS crawl carries in with the
boards it wants: healthcare, skilled trades, retail and hospitality — nearly
half of the healthcare population being Russian and completely uniform, two
hundred spellings of `Врач-…`.

This is an IT job aggregator, and these categories name work it does not exist
to serve. They are here to be FILTERABLE: a facet excludes as well as it
selects, and 225 000 unfilterable postings are worse than four options no IT
candidate will choose.

Unlike the two engineering categories, these are deliberately NOT members of
the craft set `cmd/prune` subtracts. They are exactly the non-technical
business that rule exists to remove, and leaving them out is also the only
choice that changes nothing: the same postings matched `ruleUnknown` before and
match `ruleBusiness` after.

## Requirements

### Requirement: Four consumer industries become categories

The system SHALL resolve the consumer-industry work a broad multi-industry ATS
crawl carries into four categories: `healthcare`, `skilled_trades`, `retail`
and `hospitality`. All four MUST be members of the non-technical category set,
so they are filterable but consume no LLM enrichment or embedding budget.

Aliases MUST cover the PLURAL spelling wherever the catalogue carries one.
`wordmatch` matches whole words and has no morphology, so a singular alias does
not reach a plural title — a gap invisible from the alias list, and one that
left `Automotive Mechanics` (1 292), `AUTOMOTIVE TIRE TECHNICIANS` (1 248) and
`Automotive Alignment Technicians` (490) resolving to nothing while their
singular forms resolved fine.

#### Scenario: Healthcare titles resolve

- **WHEN** a job titled "Registered Nurse", "Caregiver", "Dental Hygienist",
  "Medication Technician", "Optometrist" or "Patient Coordinator" is classified
- **THEN** its category is `healthcare`

#### Scenario: Skilled-trade titles resolve

- **WHEN** a job titled "Service Technician", "Field Service Technician",
  "Diesel Technician", "Automotive Mechanic", "Electrician", "Welder",
  "HVAC Technician" or "Installation Technician" is classified
- **THEN** its category is `skilled_trades`

#### Scenario: The plural spellings resolve too

- **WHEN** a job titled "Automotive Mechanics", "AUTOMOTIVE TIRE TECHNICIANS"
  or "Automotive Alignment Technicians" is classified
- **THEN** its category is `skilled_trades`, the same as the singular spelling

#### Scenario: Retail titles resolve

- **WHEN** a job titled "Team Member", "Retail Service Specialist", "Sales
  Associate", "Cashier", "Merchandising Specialist", "Brand Ambassador" or
  "Grocery Clerk" is classified
- **THEN** its category is `retail`

#### Scenario: Hospitality titles resolve

- **WHEN** a job titled "Server", "Host", "Chef", "Line Cook", "Barista",
  "Bartender" or "Dishwasher" is classified
- **THEN** its category is `hospitality`

#### Scenario: All four are non-technical

- **WHEN** a job resolves to any of the four
- **THEN** its derived `is_tech` is `false`, and it is not enqueued for AI
  enrichment or semantic embedding


### Requirement: The consumer categories are NOT craft-protected from deletion

`engineering_design` and `industrial_engineering` are subtracted from
`cmd/prune`'s business rule because they name a craft an IT board does not
serve but should not destroy. These four are different: they are exactly the
non-technical business the rule exists to remove at a company with no technical
history. The system SHALL NOT add them to the craft set.

This MUST be behaviour-neutral, and that is the point. Today these postings
carry no category and an unknown `is_tech`, so they match `ruleUnknown`;
afterwards they carry a non-technical category and match `ruleBusiness`. Both
rules fire only where the company has never shown technical evidence, so the
same postings are removable before and after — resolving a category must not
be what decides a posting's fate.

#### Scenario: A consumer posting is removable exactly as before

- **WHEN** the prune rules evaluate a "Server" at a company with no technical
  evidence, before and after this change
- **THEN** it matches a removal rule in both cases — `ruleUnknown` before,
  `ruleBusiness` after

#### Scenario: A consumer posting at a technical company is kept

- **WHEN** the prune rules evaluate a "Server" at a company that has posted
  technical work
- **THEN** it is kept, exactly as before

#### Scenario: The craft set is unchanged

- **WHEN** the craft set is read
- **THEN** it contains `engineering_design` and `industrial_engineering` and
  none of the four categories added here

### Requirement: The Russian medical and trade vocabularies resolve

Nearly half of the healthcare cluster is Russian and completely uniform: two
hundred spellings of `Врач-…`, none of which resolves today. The system SHALL
resolve `врач` as a bare token — the qualified forms hyphenate
(`Врач-терапевт`, `Врач-акушер-гинеколог`) or postfix a phrase (`Врач
ультразвуковой диагностики`), and a hyphen is a word boundary — together with
`медсестра`, `фельдшер` and `санитар`.

The same shape applies to the trades: `электрик`, `сварщик`, `слесарь`,
`плотник`, `механик`.

#### Scenario: The Russian medical family resolves

- **WHEN** a job titled "Врач-терапевт", "Врач-хирург", "Ветеринарный врач",
  "Врач ультразвуковой диагностики", "Медсестра" or "Фельдшер" is classified
- **THEN** its category is `healthcare`

#### Scenario: The Russian trades resolve

- **WHEN** a job titled "Электрик", "Электросварщик ручной сварки", "Слесарь",
  "Плотник" or "Электромеханик" is classified
- **THEN** its category is `skilled_trades`

### Requirement: A bare alias never claims a word that is inside another word

The Russian titles in this vocabulary are long compounds, and several contain a
shorter title inside them: `Делопроизводитель` ends in `водитель` (driver),
`Электромеханик` contains `механик` (mechanic). The system relies on whole-word
matching for this and MUST carry a regression test for each such pair, because
nothing in the alias list itself shows the hazard.

#### Scenario: A compound is not claimed by the word inside it

- **WHEN** a job titled "Делопроизводитель" is classified
- **THEN** it does NOT resolve to any category containing the driver alias

#### Scenario: Collisions between the four are decided by declaration order

- **WHEN** a job titled "Medication Technician" or "Store Driver" is classified
- **THEN** its category is `healthcare` and `retail` respectively — the
  qualified spelling wins over the bare `technician` and `driver` families

### Requirement: The consumer seats expose named roles

The system SHALL name the seats the catalogue carries in volume, so the facet
is useful beyond the coarse category: `nurse`, `caregiver`, `service_technician`,
`automotive_technician`, `electrician`, `welder`, `retail_associate`,
`cashier`, `server`, `cook` and `barista`.

#### Scenario: A consumer seat resolves to its own role

- **WHEN** a job titled "Registered Nurse", "Service Technician" or "Line Cook"
  is classified
- **THEN** its roles include `nurse`, `service_technician` and `cook`
  respectively

### Requirement: The four categories are labelled and selectable

The system SHALL label all four on every surface that renders a facet code, and
SHALL place them in the web picker. None of the existing eight groups fits a
nurse or a line cook, so a new group is required — a category absent from the
section map is generated into the contracts and unreachable by a user.

#### Scenario: The categories render and can be chosen

- **WHEN** a user opens the category filter
- **THEN** all four appear with human labels under a group that names
  consumer-industry work, and selecting one filters the job list

#### Scenario: The graded roles are nameable

- **WHEN** the role catalogue is built
- **THEN** each of the four has a role noun, so its bare and graded roles carry
  a label
