## MODIFIED Requirements

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
