## MODIFIED Requirements

### Requirement: The list toolbar no longer hosts a filter trigger

The list toolbar (`ListToolbar`) SHALL NOT render a filter trigger in either its inline
sort row or its scroll-revealed floating edge variant; the All-filters trigger is hosted
solely by the header search box. The toolbar's list controls and Swipe affordance SHALL
remain.

The toolbar's control slot SHALL carry however many list controls the hosting view
passes — on the jobs list, the sort select, the freshness select and the evergreen
toggle (see `jobs-list-controls`); on the company catalog, its own sort select. The
slot SHALL render on desktop whenever controls are passed, including on a view that
renders its result total elsewhere and therefore suppresses the toolbar's own total.
Tying the controls' visibility to the total is what hid them on a company page.

#### Scenario: No filter button in the toolbar row

- **WHEN** a user views the jobs or companies list
- **THEN** the toolbar row shows the list controls and, where applicable, the Swipe
  affordance, but no filter button

#### Scenario: No floating filter button on scroll

- **WHEN** the user scrolls the list so the toolbar leaves the viewport
- **THEN** no floating filter edge button appears; the header search box's trigger
  remains the way to open filters

#### Scenario: Controls render where the total is hosted elsewhere

- **WHEN** a view passes list controls but renders its result total outside the
  toolbar
- **THEN** the desktop toolbar row renders the controls and omits only the total
