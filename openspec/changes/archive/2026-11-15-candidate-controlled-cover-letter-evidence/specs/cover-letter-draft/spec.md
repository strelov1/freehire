## ADDED Requirements

### Requirement: Candidates control persisted cover-letter evidence

The system SHALL persist an owner-scoped selection per vacancy with optimistic revision checks,
including concurrent first saves. An absent selection SHALL preserve automatic retrieval; an
explicit empty selection SHALL remain empty. Updates SHALL reject duplicate, malformed, foreign,
non-publishable or more than 50 atom identifiers without changing the saved selection.

#### Scenario: Save and reload
- **WHEN** a candidate saves selected achievements and returns through another CV for the same vacancy
- **THEN** the same selection and revision are returned without a model call or allowance charge

#### Scenario: Conflicting save
- **WHEN** two tabs save against the same revision, including revision zero
- **THEN** only one succeeds and the other receives a conflict

### Requirement: Saved evidence constrains every drafting entry point

The endpoint and assistant SHALL resolve and freeze saved evidence before charging. Explicit
empty, deleted or non-publishable selections SHALL block generation. Explicit selections SHALL
constrain both offered atoms and achievement claims in the professional profile. Existing
provenance and citation validation SHALL still apply.

#### Scenario: Unselected achievements
- **WHEN** a candidate drafts with an explicit selection
- **THEN** unselected achievement claims are absent from all model inputs and citations

#### Scenario: Invalid selection
- **WHEN** selected evidence has been deleted or is no longer publishable, or selection is empty
- **THEN** generation is refused before charging or opening a stream and the existing letter remains

### Requirement: Draft freshness includes selected evidence

The system SHALL store the generation's selection revision and deterministic evidence fingerprint.
Changed selections or edited claims SHALL mark the letter's evidence changed. A running request
SHALL use its initial snapshot. Legacy letters SHALL remain readable with unknown evidence freshness.

#### Scenario: Evidence changes during generation
- **WHEN** a candidate changes a selection while its previous snapshot is generating
- **THEN** the completed letter retains its original snapshot metadata and is shown as changed

### Requirement: Evidence review is optional and preserves user work

The cover-letter tab SHALL provide labeled evidence checkboxes, suggestions, other confirmed
facts, a bank editor link, save and generate actions. Direct drafting SHALL honor saved choices.
Selection SHALL NOT mutate bank facts. Failed saves and conflicts SHALL retain local choices;
late responses for another vacancy SHALL NOT replace the current view. Selected facts SHALL be
distinguished from cited facts. Copy SHALL avoid claiming proof of sentence-level factual accuracy.

#### Scenario: Optional review
- **WHEN** a candidate drafts without opening review
- **THEN** the existing saved selection is honored, or automatic mode is used if none exists

#### Scenario: Empty selection
- **WHEN** all facts are unchecked
- **THEN** saving is allowed but generation is disabled with guidance to select evidence
