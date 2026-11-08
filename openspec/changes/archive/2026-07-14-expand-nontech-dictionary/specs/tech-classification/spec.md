## MODIFIED Requirements

### Requirement: Deterministic non-tech title detection

The system SHALL provide a deterministic, curated dictionary that identifies confidently non-technical job titles by whole-word match, and MUST NOT guess: a title it cannot confidently place as non-tech yields no signal. The dictionary MUST cover the high-frequency non-technical role clusters that generic ATS boards contribute — healthcare, food service, retail, warehouse/logistics, skilled trades, office/finance administration, education, and facilities — using only unambiguous role nouns, and MUST NOT contain generic terms that also occur in technical roles (e.g. bare "engineer", "technician", "analyst", "coordinator", "specialist", "administrator", "server", "warehouse", "chef").

#### Scenario: Confident non-tech title is detected

- **WHEN** a title contains a curated non-tech role noun as a whole word (e.g. "Registered Nurse", "Line Cook", "Warehouse Associate", "Paralegal", "Journeyman Electrician")
- **THEN** the detector reports the title as non-tech

#### Scenario: Technical title is not flagged

- **WHEN** a title states a technical role (e.g. "Software Engineer II, AWS DynamoDB", "IT Technician", "Data Warehouse Engineer", "Security Engineer")
- **THEN** the detector reports no non-tech signal

#### Scenario: Ambiguous substring does not match

- **WHEN** a non-tech term appears only as a substring of another word or the title carries only a tech-colliding term like bare "engineer" or "technician"
- **THEN** the detector does not flag the title, matching only on word boundaries
