## MODIFIED Requirements

### Requirement: Companies list

The frontend SHALL present companies from `GET /api/v1/companies`, showing each
company's name and its job count, with each row linking to the company detail.

The page SHALL provide a name-search input. Typing SHALL filter the list against
the API's `q` parameter (debounced), and the current query SHALL be mirrored into
the URL query string (`?q=`) so a search survives reload, sharing, and
back/forward navigation. The page SHALL show the count of matching companies and
a distinct empty state when a search matches nothing.

The page SHALL present filters through the same two-pane "All filters" modal and
chip-summary sidebar the jobs list uses (the reusable filter shells), over the
company facet set: **collection**, **region**, **country**, **industry** (domains),
**company type**, and **company size**, reusing the jobs filter controls and
closed-vocabulary option registries (country is a searchable select over the country
list; the others are pill/select controls over their fixed vocabularies). On desktop
the sidebar SHALL show the applied-facet chips plus an **All filters** button opening
the modal; on narrow viewports the modal SHALL open from a pinned left-edge tab. As
in the jobs modal, facet edits SHALL be staged and applied on **Show results**;
applying SHALL refetch the list against the corresponding repeatable API facet
parameters and mirror the active facets into the URL query string, so a filtered view
survives reload, sharing, and back/forward navigation, composably with the `q` search.
The summary sidebar SHALL offer a **Reset all** control.

#### Scenario: Companies are listed

- **WHEN** a user opens `/companies`
- **THEN** a page of companies is fetched and rendered with job counts

#### Scenario: User searches companies by name

- **WHEN** a user types a query into the companies search input
- **THEN** the list is refetched filtered by that query and the URL query string
  is updated to `?q=<query>`

#### Scenario: Search restored from the URL

- **WHEN** a user opens `/companies?q=acme` directly or via back/forward
- **THEN** the search input is prefilled with `acme` and the filtered list is
  shown

#### Scenario: Search matches nothing

- **WHEN** a search returns no companies
- **THEN** an empty state ("No matching companies.") is shown instead of an empty
  list

#### Scenario: User filters companies by a facet

- **WHEN** a user opens the **All filters** modal, selects a region, and activates
  **Show results**
- **THEN** the list is refetched against `?regions=<value>` and the URL query
  string reflects the active facet

#### Scenario: Filters restored from the URL

- **WHEN** a user opens `/companies?collections=yc&regions=europe` directly or via
  back/forward
- **THEN** the summary sidebar shows those facets active and the list is filtered to
  match

#### Scenario: Clearing filters

- **WHEN** a user activates **Reset all**
- **THEN** the facet parameters are removed from the URL and the full list (for the
  current `q`, if any) is shown

## ADDED Requirements

### Requirement: Profile filters appear only on the Market coverage tab

The `/my/profile` page SHALL expose its role/filter controls (the summary sidebar,
the mobile left-edge tab, and the two-pane modal) only while the **Market coverage**
tab is active, since coverage is the only view computed against ad-hoc filters. On the
**Your CV** and **CV readiness** tabs the filter controls SHALL NOT be shown, and CV
readiness SHALL be scored against the profile's default role. The profile page SHALL
NOT present the "My filters" (saved-search) tab.

#### Scenario: Filters shown on Market coverage

- **WHEN** a signed-in user with a profile opens the **Market coverage** tab
- **THEN** the filter summary, the mobile filters tab, and the **All filters** modal
  are available to refine the comparison role

#### Scenario: Filters hidden on other tabs

- **WHEN** the user switches to the **Your CV** or **CV readiness** tab
- **THEN** no filter summary, mobile tab, or modal is shown

#### Scenario: No My filters on the profile

- **WHEN** the profile's **All filters** modal is opened
- **THEN** it presents the facet rail without a "My filters" (saved-search) tab
