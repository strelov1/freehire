## RENAMED Requirements

- FROM: `### Requirement: Profile filters appear only on the Market coverage tab`
- TO: `### Requirement: Profile filters appear only on the CV readiness tab`

## MODIFIED Requirements

### Requirement: Profile filters appear only on the CV readiness tab

The `/my/profile` page SHALL expose its role/filter controls (the summary sidebar,
the mobile left-edge tab, and the two-pane modal) only while the **CV readiness**
tab is active. Profile no longer has a Market coverage tab — that comparison view
moved to `/my/market-pulse` — so CV readiness is now the only Profile tab whose
scoring depends on an ad-hoc role/region/seniority selection, and the filter
controls gate on it instead. On the **Settings**, **Skills**, **Profile**, and
**Experience** tabs the filter controls SHALL NOT be shown. The profile page
SHALL NOT present the "My filters" (saved-search) tab.

#### Scenario: Filters shown on Market coverage

- **WHEN** a signed-in user with a profile opens the **CV readiness** tab
- **THEN** the filter summary, the mobile filters tab, and the **All filters**
  modal are available to refine the role CV readiness is scored against

#### Scenario: Filters hidden on other tabs

- **WHEN** the user switches to **Settings**, **Skills**, **Profile**, or
  **Experience**
- **THEN** no filter summary, mobile tab, or modal is shown

#### Scenario: No My filters on the profile

- **WHEN** the profile's **All filters** modal is opened
- **THEN** it presents the facet rail without a "My filters" (saved-search) tab

#### Scenario: No Market coverage tab on Profile

- **WHEN** a signed-in user opens `/my/profile`
- **THEN** the tab strip offers Settings, Skills, Profile, Experience, and CV
  readiness, with no Market coverage tab
