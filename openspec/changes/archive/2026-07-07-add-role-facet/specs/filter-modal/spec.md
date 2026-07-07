## ADDED Requirements

### Requirement: Role is a count-driven picker alongside seniority and specialization

The filter modal SHALL present a "Role" control that lets the user pick one or
more natural roles from the derived `role` facet. The control SHALL be
count-driven (dynamic): its options come from the live facet distribution,
ordered busiest-first, with a facet-local typeahead for high cardinality, reusing
the same dynamic facet-section path as the `skills` control. Each role SHALL
render its catalog label (not the raw slug). The control SHALL support per-facet
Exclude and an AND/OR mode, consistent with the other high-cardinality facets.
The Role control SHALL be **additive**: the
existing seniority and specialization controls remain available and unchanged in
this change.

#### Scenario: Roles are offered busiest-first with labels

- **WHEN** the user opens the Role control
- **THEN** the offered roles come from the live `role` facet distribution ordered
  by descending count, each shown with its catalog label

#### Scenario: Selecting roles filters the results

- **WHEN** the user selects the "Founding Engineer" and "Staff Engineer" roles
  and applies the filters
- **THEN** the search is filtered to jobs whose `roles` include
  `founding_engineer` or `staff_engineer`

#### Scenario: Seniority and specialization controls remain

- **WHEN** the Role control is present in the modal
- **THEN** the existing seniority and specialization controls are still shown and
  usable
