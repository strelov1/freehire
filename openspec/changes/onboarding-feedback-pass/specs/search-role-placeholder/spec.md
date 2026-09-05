## ADDED Requirements

### Requirement: The jobs search box offers rotating role examples

The jobs search box SHALL show an example of what to type, phrased as
`Search jobs — e.g. <role>`, and SHALL cycle the trailing role on a fixed interval.

The roles SHALL be drawn from the generated category vocabulary as typed keys, never as
hand-written display strings, so that a category renamed or retired on the backend fails
the build rather than leaving the box offering a value the feed can no longer filter on.
Their labels SHALL resolve through the one existing category-label map.

The rotation order SHALL place the busiest role first, because under reduced motion the
first entry is the only one shown.

The companies search box SHALL keep its static placeholder — it is not a role box.

#### Scenario: The box cycles roles while untouched

- **WHEN** a visitor loads a page carrying the jobs search box and does not interact with it
- **THEN** the placeholder's trailing role advances on each interval, fading between entries

#### Scenario: A retired category fails the build

- **WHEN** a category named in the placeholder list is removed from the backend vocabulary
      and the contracts are regenerated
- **THEN** the type check fails naming that key, rather than the box continuing to offer it

### Requirement: Rotation stops at the first interaction and never restarts

The rotation SHALL stop permanently for the remainder of the visit at the first focus or
keystroke on the field, and SHALL freeze on the entry currently displayed rather than
reverting to a static string.

Text moving under the cursor while a query is being composed is the failure an animated
placeholder invites; reverting on stop would be a visible jump at the moment the visitor's
attention is on the field.

#### Scenario: Focus stops the rotation

- **WHEN** a visitor focuses the search field while it is empty
- **THEN** the placeholder stops advancing, holds the entry it was showing, and does not
      resume when focus leaves

#### Scenario: Typing stops the rotation

- **WHEN** a visitor types into the field
- **THEN** the placeholder stops advancing for the rest of the visit

### Requirement: Reduced motion disables the rotation

Where the user agent reports `prefers-reduced-motion: reduce`, the box SHALL render the
first entry and SHALL NOT start a rotation timer. The preference SHALL be re-read while
the page is open, not sampled once.

A visitor who enables the preference mid-visit SHALL keep the entry then displayed rather
than being returned to the first one. Snapping back is itself a movement, and it would
arrive at the exact moment somebody asked for less of it; the rule matches the freeze that
already applies when the rotation is stopped by an interaction.

#### Scenario: Reduced motion holds the first entry

- **WHEN** a visitor with `prefers-reduced-motion: reduce` loads the box
- **THEN** the placeholder shows the first role and never changes

#### Scenario: Enabling reduced motion mid-visit stops without a jump

- **WHEN** a visitor enables the preference while the box has already advanced
- **THEN** the rotation stops on the entry then shown, and no further entry appears

### Requirement: The field's accessible name does not move

The search field's accessible name SHALL be a static string describing the field, and SHALL
NOT be derived from the rotating placeholder.

The search component SHALL take the visible example and the accessible name as separate
inputs, and the accessible name SHALL be required of every caller rather than defaulting to
the example — a default would silently reinstate the coupling at the next call site.

#### Scenario: A screen reader announces a stable field name

- **WHEN** a screen-reader user reaches the jobs search field at any point in the rotation
- **THEN** the field is announced by its static name, not by the currently shown example
