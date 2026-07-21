## ADDED Requirements

### Requirement: Company-list filter mode

On the companies list (`/companies`), the header filter menu SHALL present a
company-appropriate scope — **Region** and **Remote hiring** pills — that
live-filters the company list through its existing filter store's `regions` and
`remote_regions` facets. Work format, countries, and cities SHALL NOT appear in
company mode (companies do not carry those facets).

#### Scenario: Companies list shows the company scope
- **WHEN** a user opens the filter menu on `/companies`
- **THEN** the popover shows Region and Remote-hiring pills and no work-format section

#### Scenario: Selecting a company region filters the list
- **WHEN** the user activates a Region or Remote-hiring pill
- **THEN** the `regions`/`remote_regions` facet updates, the company list and its counts reload, and the choice is mirrored to the URL

### Requirement: Launcher mode on listless pages

The header search launcher SHALL show the filter trigger on pages without a
filterable list (job detail, `/about`, `/collections`, …); because there is no list
to filter in place, selecting a value SHALL navigate to the jobs feed (`/jobs`) with
that scope applied, where further changes filter live.

#### Scenario: Trigger present on a listless page
- **WHEN** a user is on a page served by the global search launcher
- **THEN** the filter trigger is shown in the header search box

#### Scenario: Selecting a scope jumps to the filtered feed
- **WHEN** the user picks a work format or region from the launcher popover
- **THEN** the app navigates to `/jobs` with that value applied as a facet in the URL

### Requirement: Trigger summary covers company and empty-launcher states

The trigger's summary label SHALL reflect the active scope in every mode: company
geography (including `remote_regions`) on the companies list, and a neutral label on
a launcher with no selection.

#### Scenario: Company remote-hiring selection summarized
- **WHEN** a company Remote-hiring region is selected on `/companies`
- **THEN** the trigger label reflects that region

#### Scenario: Empty launcher shows the neutral label
- **WHEN** the launcher popover has no selection
- **THEN** the trigger shows the neutral "Location" label
