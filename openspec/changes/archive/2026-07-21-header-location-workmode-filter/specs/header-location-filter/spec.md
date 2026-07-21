## ADDED Requirements

### Requirement: Location & work-format trigger in the header search box

The header search box on jobs-backed list pages SHALL render a scope-prefix
trigger, positioned to the left of the search icon and separated from it by a
divider, that opens a Location & work-format popover. The trigger SHALL NOT alter
the search text input or its `/` focus hotkey.

#### Scenario: Trigger shown on the jobs feed
- **WHEN** a user views the jobs feed (`/`) or a company's jobs list (`/companies/:slug`)
- **THEN** the header search box shows the scope-prefix trigger before the search icon

#### Scenario: Trigger hidden where it does not apply
- **WHEN** a user views the company list (`/companies`) or any page served by the global search launcher
- **THEN** no scope-prefix trigger is shown and the header search box is unchanged

#### Scenario: Opening and dismissing the popover
- **WHEN** the user clicks the trigger
- **THEN** the popover opens; and it closes on an outside click, on Escape, or on navigation

### Requirement: Work-format and location selection drive the page filter store

The popover SHALL present a Work format pill row (Remote / Hybrid / On-site) above
the location pane (region → country accordion and searchable cities). Every
selection SHALL drive the active page's existing job-filter store facets
(`work_mode`, `regions`, `countries`, `cities`) using its established cycle
(off → include → exclude → off) semantics, with no new filter logic and no
additional facet-count request.

#### Scenario: Selecting a work format
- **WHEN** the user activates the "Remote" pill
- **THEN** the `work_mode` facet includes `remote`, the jobs list and its counts reload, and the selection is mirrored to the URL

#### Scenario: Selecting a location
- **WHEN** the user selects a region, country, or city in the pane
- **THEN** the corresponding `regions`/`countries`/`cities` facet updates and the list reloads, identically to the same selection made in the full filter modal

#### Scenario: Clearing the scope
- **WHEN** the user activates "Clear all" in the popover
- **THEN** the `work_mode`, `regions`, `countries`, and `cities` facets are all cleared

#### Scenario: Reusing already-fetched facet counts
- **WHEN** the popover renders the location pane
- **THEN** it uses the jobs view's already-fetched facet counts and issues no new facet-count request

### Requirement: Trigger label summarizes the current scope

The trigger SHALL display a computed summary of the current work-format and
geography selection so the active scope is visible without opening the popover.
On narrow viewports the text summary SHALL collapse to an icon.

#### Scenario: No selection
- **WHEN** no work-format or geography facet is selected
- **THEN** the trigger reads a neutral "Location" label

#### Scenario: Work format only
- **WHEN** only a work format is selected (e.g. Remote)
- **THEN** the trigger shows that format's icon and label

#### Scenario: Geography only, multiple values
- **WHEN** two or more geography values are selected (e.g. Europe and UK and APAC)
- **THEN** the trigger shows the first value's label followed by a "+N" roll-up of the rest

#### Scenario: Both work format and geography
- **WHEN** both a work format and one or more geography values are selected
- **THEN** the trigger shows the format followed by the geography summary (e.g. "Remote · Europe +1")
