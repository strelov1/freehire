## MODIFIED Requirements

### Requirement: Freshness is selectable above the list

The jobs list SHALL render one date control above the list, offering the same stops
the filter modal offers for the bound it writes, and defaulting to no bound. Selecting
a stop SHALL apply immediately rather than on a debounce, because a select is a
discrete choice and not a dragged gesture.

The control SHALL bound how long a posting has been open — the date the system first
recorded it — and SHALL be labelled **Open**. Of the two date bounds the modal offers,
this is the one no source can rewrite, and so the only one that can answer the question
the control's placement implies. A posting whose source restates its date every crawl
would otherwise pass a three-day bound while its own card reads that it has been open
for months.

When the runtime feature flag for the open bound is off, the control SHALL instead
render as **Posted**, writing the source-stated bound as it did before. There SHALL NOT
be a state in which the control is absent: the flag governs which date is bounded above
the list, never whether a date can be bounded there at all.

The control SHALL write to the same filter state the corresponding modal slider writes
to, so the two, the filter summary and a saved search all read one value and cannot
drift.

A bound that is not one of the stops — from a shared link, a hand-edited URL, or
the AI filter dialog, which writes whatever day count it read — SHALL be offered
as a stop of its own, in day order, labelled for what it is. A select can only
show a value it has an option for; without one the control renders blank while
the bound quietly narrows the list, which is the same untruth the freshness label
already refuses to tell.

#### Scenario: Choosing a stop bounds the list

- **WHEN** the user selects `1 week` in the control with the open-bound flag enabled
- **THEN** the list reloads bounded to that window and the URL carries
  `open_within_days=7`

#### Scenario: The control bounds the source date while the flag is off

- **WHEN** the open-bound flag is off and the user selects `1 week`
- **THEN** the control is labelled `Posted` and the URL carries
  `posted_within_days=7`

#### Scenario: A rewritten posting date does not pass the above-list bound

- **WHEN** a posting first recorded 72 days ago states a posting date of today, and the
  user selects a `3 days` stop with the flag enabled
- **THEN** that posting is not in the list

#### Scenario: The modal reflects a stop chosen above the list

- **WHEN** the user selects a stop above the list and then opens the filter modal
- **THEN** the modal's control for that same bound shows the same stop

#### Scenario: Clearing the bound restores the full list

- **WHEN** the user selects `Any`
- **THEN** the URL carries no key for that bound

#### Scenario: An off-preset bound is shown rather than left blank

- **WHEN** a link carrying an off-preset day count for the bound is opened
- **THEN** the control offers a stop of its own for that count, in day order, and
  shows it as selected

## REMOVED Requirements

### Requirement: Likely-evergreen postings can be hidden from the list

**Reason**: The toggle wrote one sign of one value of the `reality` facet — a filter
wearing a button's clothes — and it duplicated a control that now sits beside the date
bounds it belongs with. It was also the widest control in a toolbar measured overflowing
a 390px viewport, and removing it is what makes room for the `Open` select.

**Migration**: The full `reality` facet is rendered in the filter modal's `Posted` pane,
beneath the two date bounds. Excluding `likely-evergreen` there writes exactly the
`reality_exclude=likely-evergreen` parameter the toggle wrote, so shared links, saved
searches and the URL contract are unchanged; only the control's location moves. No
parameter is removed, renamed, or redefined.
