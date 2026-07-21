## ADDED Requirements

### Requirement: All-filters trigger in the header search box

On list pages backed by a filter modal — the jobs feed (`/`), a company's jobs list
(`/companies/[slug]`), a collection landing page (`/collections/[slug]`), and the
companies list (`/companies`) — the shared header search box SHALL render an **All
filters** trigger at its right edge, after the clear control and mirroring the location
scope-prefix on the left. The trigger SHALL be shown on every viewport. Activating it
SHALL open that page's own filter modal (the jobs `FilterModal` or the companies
`CompanyFilterModal`) without changing the search text or its focus hotkey. The trigger
SHALL NOT appear on pages served by the global search launcher or on the `/my/profile`
page, whose Market-coverage filter tab is unaffected.

#### Scenario: Trigger shown on a jobs-backed list

- **WHEN** a user views the jobs feed (`/`), a company's jobs list (`/companies/:slug`),
  or a collection landing page (`/collections/:slug`)
- **THEN** the header search box shows an All-filters trigger at its right edge on every
  viewport

#### Scenario: Trigger shown on the companies list

- **WHEN** a user views the companies list (`/companies`)
- **THEN** the header search box shows an All-filters trigger that opens the companies
  filter modal

#### Scenario: Activating the trigger opens the page's modal

- **WHEN** the user activates the All-filters trigger
- **THEN** the active page's filter modal opens, and the search text and its `/` focus
  hotkey are unchanged

#### Scenario: Trigger hidden where no filterable list exists

- **WHEN** a user is on a page served by the global search launcher, or on `/my/profile`
- **THEN** no All-filters trigger is shown in the header search box

### Requirement: The trigger reflects the active-filter count

The All-filters trigger SHALL display a badge with the number of currently active filters
for the page, and SHALL show no badge when no filters are active. The count SHALL update
reactively as filters are applied or cleared.

#### Scenario: Badge shows the active count

- **WHEN** two filters are active on the current list
- **THEN** the trigger's badge shows `2`

#### Scenario: No badge when nothing is filtered

- **WHEN** no filters are active on the current list
- **THEN** the trigger shows no count badge

### Requirement: The list toolbar no longer hosts a filter trigger

The list toolbar (`ListToolbar`) SHALL NOT render a filter trigger in either its inline
sort row or its scroll-revealed floating edge variant; the All-filters trigger is hosted
solely by the header search box. The toolbar's sort control and Swipe affordance SHALL
remain.

#### Scenario: No filter button in the toolbar row

- **WHEN** a user views the jobs or companies list
- **THEN** the toolbar row shows the sort control and, where applicable, the Swipe
  affordance, but no filter button

#### Scenario: No floating filter button on scroll

- **WHEN** the user scrolls the list so the toolbar leaves the viewport
- **THEN** no floating filter edge button appears; the header search box's trigger
  remains the way to open filters
