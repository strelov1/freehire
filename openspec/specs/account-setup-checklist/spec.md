# account-setup-checklist Specification

## Purpose

Surfaces a checklist of outstanding account-setup steps to a signed-in user, on the page where those steps are actually completed, and lets each outstanding step be opened directly.

## Requirements

### Requirement: Checklist surfaces on the profile page

The system SHALL show the "finish setting up your account" checklist on the profile page, above the profile's section tabs, whenever the signed-in user has a saved profile and at least one setup step is outstanding. The checklist SHALL NOT appear on the profile page before a profile has been saved, and SHALL NOT appear once every step is complete.

The checklist SHALL NOT be shown on the tracking page.

#### Scenario: Incomplete account shown on the profile page

- **WHEN** a signed-in user with a saved profile and at least one outstanding setup step opens the profile page
- **THEN** the checklist is displayed above the profile's section tabs, listing every step and how many are done

#### Scenario: Complete account shows no checklist

- **WHEN** a signed-in user whose profile has every setup step complete opens the profile page
- **THEN** the checklist is not displayed

#### Scenario: No checklist before a profile exists

- **WHEN** a signed-in user who has not yet saved a profile opens the profile page
- **THEN** the checklist is not displayed (only the profile creation form is shown)

#### Scenario: Tracking page no longer shows the checklist

- **WHEN** a signed-in user with outstanding setup steps opens the tracking page
- **THEN** the checklist is not displayed there, regardless of how many steps are outstanding

### Requirement: Outstanding steps open their own profile tab

For each outstanding checklist step whose remaining action is completed on the profile page, the system SHALL link that step directly to the profile section (tab) where the missing information is entered, rather than to the profile page's default section. Following the link SHALL select that section whether the user is navigating to the profile page for the first time or is already on the profile page viewing a different section.

#### Scenario: Opening the skills step from a fresh navigation

- **WHEN** a user with no listed skills opens the checklist from elsewhere and follows the "List your skills" step
- **THEN** the profile page opens with its Skills section selected

#### Scenario: Opening the location step while already on the profile page

- **WHEN** a user with no stated location preference is on the profile page viewing an unrelated section and follows the "Set where and how you want to work" step from the checklist
- **THEN** the profile page's Location section becomes selected without a full page reload

#### Scenario: Opening the role step

- **WHEN** a user missing a specialization or seniority follows the "Say what you do, and at what level" step
- **THEN** the profile page opens with the section that holds role and seniority selected

#### Scenario: Steps outside the profile page are unaffected

- **WHEN** a user follows the "Add your CV" or "Get new matches sent to you" checklist steps
- **THEN** they are taken to the CV and search-alerts pages as before, unaffected by tab targeting
