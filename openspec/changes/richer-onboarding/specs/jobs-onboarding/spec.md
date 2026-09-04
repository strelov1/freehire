## ADDED Requirements

### Requirement: The wizard captures the candidate beyond their filters

The system SHALL present, in the post-registration wizard, steps covering: the résumé, the
candidate's specializations together with their public profile links, their years of
experience, their skills, their geography, their money (current income and desired salary),
their job-search stage, and their single biggest challenge.

Steps that constrain what the candidate is shown come before steps that only describe them,
so that a run abandoned part-way has still answered the questions the product can act on.
Every step remains skippable, and skipping one MUST NOT prevent reaching any later one.

Each answer is written to the store that already owns that fact rather than to a store built
for the wizard: search preferences to the search profile, desired salary to the screening
answers, years of experience and profile links to the candidate-owned résumé overlay, and
the three segmentation answers to the questionnaire. The wizard MUST NOT introduce a second
copy of a fact one of those stores already holds.

#### Scenario: A full run reaches every store

- **WHEN** a user answers every step of the wizard
- **THEN** the search profile, the screening answers, the résumé overlay, and the questionnaire each hold the answers belonging to them

#### Scenario: Skipping a step does not block the rest

- **WHEN** a user skips the skills step
- **THEN** the wizard advances to the next step and every later step remains reachable

#### Scenario: The money step writes one desired salary, not two

- **WHEN** a user states a desired salary in the wizard
- **THEN** it is stored as the account's existing desired-salary answer, and no second desired-salary value exists anywhere

### Requirement: Each step persists as it is answered

The system SHALL persist a step's answer when the user leaves that step, not when the wizard
finishes. A user who abandons the wizard part-way MUST keep every answer given before the
step they abandoned on.

A failure to persist one step MUST NOT discard the answers already stored by earlier steps.

#### Scenario: Abandoning mid-run keeps earlier answers

- **WHEN** a user answers the first five steps and then navigates away
- **THEN** those five answers are stored, and the remaining questions are unanswered

#### Scenario: Resuming shows only what is unanswered

- **WHEN** a user who abandoned mid-run re-enters the wizard
- **THEN** the steps they already answered are not asked again

### Requirement: Onboarding completion is an explicit account fact

The system SHALL record whether an account has been through onboarding as an explicit,
server-side fact on the account, and SHALL decide whether to route a signed-in user into the
wizard from that fact alone. Completion MUST NOT be inferred from any other stored artefact.

The system SHALL mark onboarding complete when the user reaches the end of the wizard, and
when the user explicitly declines to continue. Merely navigating away MUST NOT mark it
complete: that user is asked again on a later visit, while a per-visit guard prevents the
route from re-capturing them within the same visit.

An account marked complete MUST NOT be routed into the wizard again, regardless of how many
questions it left unanswered.

#### Scenario: An account that has never onboarded is routed in

- **WHEN** a signed-in user whose account is not marked complete opens the site
- **THEN** they are routed into the wizard

#### Scenario: An existing account with a résumé is still routed in

- **WHEN** a signed-in user who already has a résumé but has never been marked complete opens the site
- **THEN** they are routed into the wizard, and it presents only the steps they have not answered

#### Scenario: Declining ends the routing for good

- **WHEN** a user explicitly declines the wizard
- **THEN** their account is marked complete and they are never routed into the wizard again

#### Scenario: Navigating away defers rather than completes

- **WHEN** a user leaves the wizard by navigating elsewhere without declining
- **THEN** their account is not marked complete, they are not re-routed during that visit, and they are routed in again on a later visit

## MODIFIED Requirements

### Requirement: Onboarding preferences persist locally across visits

The system SHALL persist the wizard's resulting filter set to local browser storage using the existing job-filter persistence mechanism.

Onboarding completion is NOT local state: it is an explicit fact on the account (see
"Onboarding completion is an explicit account fact"), so a user who completed onboarding is
not asked again in a different browser, and a browser whose storage is cleared does not
re-ask a completed account. Local storage retains only the feed banner's own
dismissed/not-dismissed nudge, which is a per-browser presentation concern.

Filter persistence MUST NOT require authentication and MUST NOT write to any server or account.

#### Scenario: Configured feed survives a return visit

- **WHEN** a visitor completes the wizard and later returns to the standalone jobs feed in the same browser
- **THEN** the feed is restored to the configured preferences

#### Scenario: Banner does not reappear after completion

- **WHEN** a visitor has completed the wizard
- **THEN** the banner is not shown on subsequent visits, while the persistent entry point remains available

#### Scenario: Completion follows the account, not the browser

- **WHEN** a user who completed onboarding signs in from a different browser
- **THEN** they are not routed into the wizard
