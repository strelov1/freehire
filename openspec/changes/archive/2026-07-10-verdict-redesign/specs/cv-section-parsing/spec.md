## ADDED Requirements

### Requirement: Section-aware CV skill extraction

The system SHALL segment a CV's plain text into a **Skills section** and the
**body** (everything outside the Skills section) by locating section headings
(EN and RU), then skill-tag each segment independently. It SHALL produce three
skill-slug sets: `declared` (skills found inside the Skills section), `body`
(skills found in the rest of the CV), and `all` (their union). The segmentation
SHALL be pure and require no LLM, and SHALL be reproducible (same CV text ⇒ same
sets). When no Skills-section heading is found, `declared` SHALL be empty and all
tagged skills SHALL fall into `body` (never a crash).

#### Scenario: Skills declared and also used in experience
- **WHEN** a CV has a "Skills" section listing "go, kubernetes" and an experience bullet that also mentions "go"
- **THEN** `declared` contains {go, kubernetes}, `body` contains {go}, and `all` contains {go, kubernetes}

#### Scenario: Skill used in experience but not declared
- **WHEN** a CV mentions "kafka" only inside an experience bullet and never in the Skills section
- **THEN** `kafka` is in `body` and not in `declared`

#### Scenario: No Skills-section heading present
- **WHEN** a CV has no recognizable Skills-section heading
- **THEN** `declared` is empty and every tagged skill is in `body`

#### Scenario: Deterministic
- **WHEN** the same CV text is parsed twice
- **THEN** the `declared`, `body`, and `all` sets are identical
