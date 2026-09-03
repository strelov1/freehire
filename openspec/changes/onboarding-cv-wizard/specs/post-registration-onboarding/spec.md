## Purpose

Gets a signed-in user's CV and search profile filled in by prompting for them right after sign-in, on every visit, until a CV exists — rather than relying on the user to find the profile page on their own.

## ADDED Requirements

### Requirement: The onboarding page is gated on CV absence

The web app SHALL redirect any authenticated user who has no CV to a dedicated full-screen `/onboarding` page from any other route, and SHALL NOT redirect an authenticated user who has a CV or an anonymous visitor there. CV presence is determined by `GET /api/v1/me/resume`'s `present` field. The redirect condition SHALL be re-evaluated on every navigation, so it fires again on a later visit if the user still has no CV — there is no separate "onboarding completed" state that suppresses it once shown. A visit that reaches `/onboarding` and then leaves it (by any of the ways described below) SHALL NOT be redirected back for the remainder of that visit, even if the account still has no CV — only a later, separate visit re-triggers the redirect.

#### Scenario: Redirected to the onboarding page without a CV
- **WHEN** a signed-in user with no CV navigates to any page
- **THEN** they are redirected to `/onboarding`

#### Scenario: Not redirected once a CV exists
- **WHEN** a signed-in user who has a CV navigates to any page
- **THEN** they are not redirected to `/onboarding`

#### Scenario: Not redirected as an anonymous visitor
- **WHEN** a signed-out visitor navigates to any page, including `/onboarding` directly
- **THEN** they are not kept on `/onboarding` (redirected away if they land there), regardless of CV state

#### Scenario: The redirect fires again on a later visit
- **WHEN** a signed-in user leaves `/onboarding` without uploading a CV, then later starts a new visit (e.g. a fresh page load)
- **THEN** they are redirected to `/onboarding` again

#### Scenario: The redirect does not fire again within the same visit
- **WHEN** a signed-in user leaves `/onboarding` without uploading a CV and then navigates to another page within the same visit
- **THEN** they are not redirected back to `/onboarding` for the rest of that visit

### Requirement: The onboarding page is a skippable three-step wizard

The page SHALL present three steps — CV upload, confirm (role, skills, level), and location — in that order, each independently skippable via a visible "Skip" control that advances to the next step (or, on the last step, leaves the page) without requiring the step's input. The page MAY also be left entirely (without completing remaining steps) via a close control, which SHALL commit whatever was staged in completed steps exactly as reaching the end would.

#### Scenario: Skipping the CV step advances to confirm
- **WHEN** a user on the CV step activates "Skip"
- **THEN** the page advances to the confirm step without a CV having been uploaded

#### Scenario: Skipping every step still leaves the page
- **WHEN** a user skips all three steps in sequence
- **THEN** the user is taken off `/onboarding` and no profile fields are sent to the server

#### Scenario: Closing early commits staged progress
- **WHEN** a user completes the confirm step, then activates the close control before reaching the location step
- **THEN** the specializations, skills, and seniorities staged on the confirm step are saved to the user's profile

### Requirement: CV upload extracts and pre-fills the confirm step

The CV step SHALL let the user upload a résumé file, using the same extraction the profile form uses. A successful extraction SHALL pre-fill the confirm step's role (specializations), skills, and level (seniorities) selections with the extracted values, merged into (not replacing) any values already staged.

#### Scenario: Extraction pre-fills the confirm step
- **WHEN** a user uploads a CV on the CV step and extraction resolves specializations, skills, and a seniority
- **THEN** the confirm step opens with those values pre-selected, editable before saving

### Requirement: A CV upload does not remove the user from the onboarding page

Uploading a CV on the CV step marks the account as having a CV, which is also this page's own gating condition (see "The onboarding page is gated on CV absence"). That change in gating state, taken by itself, SHALL NOT redirect the user off `/onboarding` mid-visit — they SHALL remain on the page (free to continue to the confirm and location steps) until they leave it themselves, through any of the ways described elsewhere in this capability.

#### Scenario: Uploading a CV keeps the user on the page
- **WHEN** a user on the CV step uploads a CV and extraction succeeds
- **THEN** the user remains on `/onboarding`, now on (or able to advance to) the confirm step

### Requirement: The confirm step offers role, skills, and level as independent multi-select fields

The confirm step SHALL present three independently multi-selectable fields: role (backed by the specialization/category vocabulary, searchable by name), skills (backed by the skills vocabulary, searchable by name), and level (backed by the seniority vocabulary, allowing more than one level to be selected). None of the three SHALL be required to advance past this step. Each field SHALL offer a control that clears its entire selection at once, shown only once that field has at least one value selected, distinct from removing one selected value at a time.

#### Scenario: Multiple roles and levels can be selected together
- **WHEN** a user on the confirm step selects two role values and two level values
- **THEN** both role values and both level values remain selected simultaneously

#### Scenario: Clearing a field removes every value it holds
- **WHEN** a user on the confirm step has two roles selected and activates that field's clear control
- **THEN** the role field holds no selected values, and the skills and level fields are unaffected

### Requirement: The confirm and location steps pre-fill from the existing profile

When the user already has a saved profile, the confirm step's role/skills/level SHALL pre-fill from the saved `specializations`/`skills`/`seniorities`, and the location step SHALL pre-fill from the saved `location_preferences`, so that a user who filled in these fields on a prior visit (but still lacks a CV, and so is redirected to the page again) does not have to re-enter them.

#### Scenario: A returning user with a partial profile sees it pre-filled
- **WHEN** a user who previously saved specializations and skills (but never uploaded a CV) is redirected to `/onboarding` again
- **THEN** the confirm step opens with those specializations and skills already selected

### Requirement: The wizard commits once, through the existing profile save

The wizard SHALL stage the CV upload, confirm-step selections, and location selection locally across steps, and SHALL commit the confirm-step and location values in a single `PUT /api/v1/me/profile` call when the user leaves `/onboarding` (by completion, skip-to-end, or early dismissal) — reusing the existing profile save; the wizard MUST NOT introduce a separate persistence path. The CV file itself is persisted by the existing CV upload/extraction call at the moment it is uploaded, independent of the confirm/location commit. Unless BOTH `specializations` and `skills` end up with at least one value, the profile save SHALL be skipped entirely (the existing save endpoint rejects either being empty), leaving any existing profile unchanged.

#### Scenario: Completing the wizard saves the profile once
- **WHEN** a user completes all three steps with valid selections
- **THEN** exactly one `PUT /api/v1/me/profile` request is sent, carrying the confirm-step and location values

#### Scenario: An empty confirm step skips the profile save
- **WHEN** a user uploads no CV, selects no role and no skills on the confirm step, and leaves the page
- **THEN** no `PUT /api/v1/me/profile` request is sent

#### Scenario: A role with no skills also skips the profile save
- **WHEN** a user selects a role but no skills (either because they picked none or the skills dictionary offered none), and leaves the page
- **THEN** no `PUT /api/v1/me/profile` request is sent — the save endpoint would reject an empty skills set regardless of the role
