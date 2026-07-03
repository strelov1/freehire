## MODIFIED Requirements

### Requirement: Filter options are selected as chips with per-facet Exclude and Clear

Every multi-value facet control in the modal SHALL render its options as chips (the
shared pill primitive), not checkboxes or radio buttons, and SHALL let a single facet
hold included and excluded values at the same time. Each facet value carries one of
three states — unselected, included, or excluded — held in the facet's separate
`include` and `exclude` sets. A pills facet SHALL cycle a value through
unselected → included → excluded → unselected on successive activations, showing the
active (filled) style when included and the destructive (red) style when excluded. A
select (searchable) facet SHALL add a picked value to the include set, render each
selected value as a chip carrying a control that toggles it between include and
exclude, and group excluded chips under the destructive style. Included values within
a facet SHALL be ORed by default, with an optional match-all (AND) toggle shown once
more than one value is included; excluded values SHALL always be ANDed (a job matches
only if it has none of them). Each facet with a selection SHALL offer a Clear control
that resets both sets. These per-value controls SHALL match the sidebar's facet
controls, and the whole-facet Exclude toggle SHALL be removed.

#### Scenario: Options render as chips

- **WHEN** a facet with a fixed option set is shown in the modal
- **THEN** its options render as chips, and an included option shows the active chip
  style while an excluded option shows the destructive (red) style

#### Scenario: A pills value cycles through include and exclude

- **WHEN** the user activates the same pills-facet value three times in a row
- **THEN** the value moves unselected → included → excluded → unselected, and the chip
  style tracks each state

#### Scenario: A select value toggles between include and exclude

- **WHEN** a select facet has a value staged in its include set and the user activates
  that chip's include/exclude toggle
- **THEN** the value moves from the include set to the exclude set (and renders under
  the destructive style), without being removed from the facet

#### Scenario: Include and exclude coexist in one facet

- **WHEN** the user includes one value and excludes another in the same facet (e.g.
  include `nodejs`, exclude `php`)
- **THEN** both selections are retained and serialize to `?<param>=nodejs` and
  `?<param>_exclude=php` in the same request

#### Scenario: Match-all toggle applies to included values only

- **WHEN** a facet has two or more included values
- **THEN** a match-all (AND) toggle is shown for the include set, and excluded values
  are unaffected by it (always ANDed)

#### Scenario: Clear resets both sets

- **WHEN** a facet has both included and excluded values and the user activates Clear
- **THEN** the facet's include and exclude sets are both emptied
