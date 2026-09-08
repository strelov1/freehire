## MODIFIED Requirements

### Requirement: The catalogue is filtered on facets, not on text

The catalogue SHALL be filterable by specialization, category, seniority, skills,
timezone region, city and years of experience. Every filter SHALL take values from a closed
vocabulary or a number; the catalogue SHALL NOT accept a free-text query.

Specialization and category are different questions over the same vocabulary:
specialization is what the candidate ticked on their own profile — where they want to go —
while category is derived from their most recent job title, which is where they have been.

**Every filter the API reads SHALL have a control on the listing surface.** A filter
reachable only by editing the URL is indistinguishable from one that does not exist, which
is the failure the catalogue was built to fix — it must not be reintroduced one facet at a
time.

The controls for the OPEN vocabularies — skills and city — SHALL be searchable and SHALL
show how many members stand behind each value. A vocabulary of thousands rendered as a row
of pills is not a longer control, it is the wrong one; and a value offered without a count
cannot be told apart from one nobody carries.

There is no language filter, because there is no language data — see the withholding rule
above. A filter over a field the card does not carry would be a filter nobody could act on.

A filter that is absent SHALL be treated identically to one that is present and empty.
Multiple values within one filter SHALL match a member carrying ANY of them; different
filters SHALL narrow together.

An unreadable filter parameter SHALL be reported in `meta.ignored_params` rather than
silently widening the answer, matching the rule the rest of this API follows.

#### Scenario: Every filter has a control

- **WHEN** a visitor opens the catalogue's filters
- **THEN** each of specialization, category, seniority, skills, timezone region, city and
  years can be set without editing the URL

#### Scenario: Searching an open vocabulary

- **WHEN** a visitor types into the skills control
- **THEN** matching skills are offered, each with the number of members carrying it
- **AND** a skill nobody in the catalogue carries is not offered

#### Scenario: Two values in one filter

- **WHEN** the catalogue is filtered by two skills
- **THEN** a member carrying either skill appears

#### Scenario: Two different filters

- **WHEN** the catalogue is filtered by a skill and a seniority
- **THEN** only members carrying both appear

#### Scenario: An empty filter

- **WHEN** a filter is present with no values
- **THEN** the answer is identical to the same request with that filter absent

#### Scenario: A filter nobody reads

- **WHEN** a request carries a parameter the catalogue does not understand
- **THEN** the parameter is named in `meta.ignored_params`
- **AND** the answer is the same as it would have been without it

#### Scenario: Specialization and category disagree

- **WHEN** a member's most recent role resolves to `backend` and their profile declares
  `ml_ai`
- **THEN** filtering by specialization `ml_ai` returns them
- **AND** filtering by category `ml_ai` does not

#### Scenario: A member with no timezone, under a timezone filter

- **WHEN** the catalogue is filtered by timezone region and a member has no timezone
- **THEN** that member is excluded
- **AND** the surface says that the filter excludes members whose timezone is unknown

#### Scenario: A narrowed catalogue is a link

- **WHEN** a visitor sets filters through the controls
- **THEN** the filters are in the URL
- **AND** opening that URL fresh, or arriving at it through the back button, shows the same
  narrowed catalogue
