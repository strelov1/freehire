# tech-classification — delta

## MODIFIED Requirements

### Requirement: Deterministic tech title detection

The system SHALL provide a deterministic, curated dictionary that identifies confidently technical (software/IT) job titles by whole-word match, and MUST NOT guess: a title it cannot confidently place as technical yields no signal.

A term is admissible when it is **anchored to the craft**, and the anchor may be any of: the word `software`; a named language, framework or vendor platform (`golang`, `flutter`, `mulesoft`, `pega`); the noun `IT` naming the estate the role runs; or a seniority word that makes the phrase unambiguous as a whole (`senior developer` — which cannot occur inside `Senior Business Developer`, since the two words are not adjacent there). A term is INADMISSIBLE when it is generic enough to be dominated by non-software roles: bare `engineer` (mechanical, manufacturing, civil, drainage), bare `analyst`, bare `developer` (business, real-estate, and the placement counsellor titled `Job Developer`), bare `architect`, bare `administrator`, and the bare abbreviation `engr` (which the catalogue carries as `Project Engr II` and `Field Service Engr II`).

**Surface form is part of the term, not a detail of it.** Whole-word matching cannot see past a word boundary, so a plural, an abbreviation and a hyphenation are each their own entry: `software engineers` does not match on `software engineer`, and `software engr` does not match on either. The dictionary SHALL carry the surface forms the catalogue actually uses rather than one canonical spelling per role.

**The dictionary's gaps SHALL be discovered from production titles, never from memory.** A test written against the same list the dictionary holds can only confirm what is already there; what is missing is visible only in the postings the detector failed to recognise. Measured 2026-09-23: 2,232,773 open canonical postings carried no `is_tech` signal, 71,314 of them on titles carrying an outright software or IT signal.

#### Scenario: Confident tech title is detected
- **WHEN** a title contains a curated software/IT role term as a whole word (e.g. "Senior Software Engineer", "Web3 Developer", "System Administrator")
- **THEN** the detector reports the title as technical

#### Scenario: A vendor-platform developer title is detected
- **WHEN** a title names an enterprise platform in the developer position (e.g. "Mulesoft Developer", "Pega Developer", "Power Platform Developer", "Flutter Developer", "SQL Developer")
- **THEN** the detector reports the title as technical, the platform name serving as the anchor

#### Scenario: A level-qualified developer title is detected
- **WHEN** a title reads "Senior Developer", "Lead Developer" or "Junior Developer"
- **THEN** the detector reports the title as technical

#### Scenario: A non-software role keeping the word developer is not flagged
- **WHEN** a title reads "Business Developer", "Senior Business Developer", "Job Developer", "Product Developer" or "Project Developer"
- **THEN** the detector reports no tech signal, because the level-qualified terms are phrases and the qualifying word is not adjacent to "developer" in them

#### Scenario: An IT-anchored role is detected
- **WHEN** a title names a role against the IT estate (e.g. "IT Officer", "IT Supervisor", "IT Trainer", "Head of IT")
- **THEN** the detector reports the title as technical

#### Scenario: The abbreviated and plural surface forms are detected
- **WHEN** a title reads "Software Engr II", "Advanced Software Engr", "Software Engineers" or "Data Engineers"
- **THEN** the detector reports the title as technical

#### Scenario: The bare abbreviation is not a term
- **WHEN** a title reads "Project Engr II" or "Field Service Engr II"
- **THEN** the detector reports no tech signal, because `engr` alone is not anchored to the craft

#### Scenario: Non-software engineering title is not flagged
- **WHEN** a title names a non-software engineering or non-tech role (e.g. "Senior Mechanical Engineer", "Professional Engineer - Drainage", "Sales Engineer", "Senior Geologist")
- **THEN** the detector reports no tech signal

#### Scenario: Ambiguous substring does not match
- **WHEN** a tech term appears only as a substring of another word or the title carries only a shared term like bare "engineer"
- **THEN** the detector does not flag the title, matching only on word boundaries
