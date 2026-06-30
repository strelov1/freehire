## ADDED Requirements

### Requirement: Source adapters may emit structured facet signals

The normalized `Job` a source adapter returns SHALL carry optional structured
facet fields — `Seniority`, `Category`, `Skills`, and `ExperienceYearsMin` —
alongside the existing `WorkMode`. Each field carries a STRUCTURED signal the
source states explicitly (an enum, tag list, or numeric field), never a value an
adapter inferred from free text. An adapter that has no structured signal for a
field SHALL leave it empty/nil, so the downstream derivation falls back to the
dictionaries. An adapter SHALL map a source value into freehire's controlled
vocabulary and SHALL emit nothing for a value it cannot map (it never guesses or
forwards an out-of-vocabulary value).

#### Scenario: An adapter with no structured signal leaves the fields empty

- **WHEN** an adapter normalizes a posting whose platform exposes no grade, skills,
  category, or experience field
- **THEN** the returned `Job` has empty `Seniority`/`Category`/`Skills` and a nil
  `ExperienceYearsMin`

#### Scenario: An unmappable source value is dropped, not forwarded

- **WHEN** an adapter reads a source facet value with no equivalent in freehire's
  controlled vocabulary
- **THEN** the corresponding `Job` field is left empty rather than carrying the raw value

### Requirement: getmatch maps its detail data into structured facets

The getmatch adapter SHALL populate the `Job`'s structured facet fields from the
per-offer detail response it already fetches, under freehire's vocabularies:
the scalar `seniority` grade SHALL map into `SeniorityValues` (unknown grades
dropped); `required_years_of_experience` SHALL set `ExperienceYearsMin` (nil when
absent); `skills_objects` names SHALL be canonicalized through the `skilltag`
dictionary, keeping only resolved skills; and `specializations` SHALL map into
`CategoryValues` via an explicit subset map, resolving to a single category or, if
the offer's specializations map to more than one distinct category, to empty.

#### Scenario: A getmatch grade becomes a structured seniority

- **WHEN** a getmatch offer's detail has `seniority` set to a grade in freehire's
  vocabulary (e.g. `senior`)
- **THEN** the normalized `Job` carries `Seniority="senior"`

#### Scenario: getmatch skills are canonicalized and noise is dropped

- **WHEN** a getmatch offer's `skills_objects` contain both a known technology
  (e.g. "Golang") and an unrecognized token (e.g. "Kiss")
- **THEN** the normalized `Job`'s `Skills` contain the canonical skill and exclude the noise

#### Scenario: Conflicting specializations resolve to empty

- **WHEN** a getmatch offer's `specializations` map to two different categories
- **THEN** the normalized `Job`'s `Category` is empty
