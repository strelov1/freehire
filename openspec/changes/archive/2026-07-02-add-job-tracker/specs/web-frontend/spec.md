## ADDED Requirements

### Requirement: My jobs page

The web SPA SHALL provide a `/my/jobs` page for signed-in users listing their
job interactions with All / Saved / Applied tabs (with per-tab counts), reusing
the standard job-row presentation and linking each row to the job page. The
page SHALL be reachable from the user menu. A signed-out user navigating to it
SHALL be prompted to sign in instead of seeing an error.

#### Scenario: Viewing applications

- **WHEN** a signed-in user opens `/my/jobs` and selects the Applied tab
- **THEN** the SPA lists the jobs they marked applied, most recently touched
  first, each row linking to the job page

#### Scenario: Tab counts

- **WHEN** the page loads
- **THEN** each tab shows the count of interactions it would list

#### Scenario: Signed-out visitor

- **WHEN** a signed-out user navigates to `/my/jobs`
- **THEN** the SPA shows a sign-in prompt, not the listing and not an error page

### Requirement: Save toggle on the job page

The web SPA SHALL show a Save/Saved toggle on the job detail page for signed-in
users, reflecting the saved state returned by the silent view recording and
flipping via the save/unsave endpoints. Signed-out users SHALL NOT see the
toggle.

#### Scenario: Saving from the job page

- **WHEN** a signed-in user clicks Save on a job page
- **THEN** the SPA calls the save endpoint and the button reflects the saved
  state from the response

#### Scenario: Unsaving

- **WHEN** a signed-in user clicks the toggle on an already-saved job
- **THEN** the SPA calls the unsave endpoint and the button returns to the
  unsaved state

#### Scenario: Signed-out user

- **WHEN** a signed-out user opens a job page
- **THEN** no Save toggle is rendered
