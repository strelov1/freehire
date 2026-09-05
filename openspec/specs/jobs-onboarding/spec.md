# jobs-onboarding Specification

## Purpose
TBD - created by archiving change jobs-onboarding-wizard. Update Purpose after archive.
## Requirements
### Requirement: Onboarding wizard captures feed preferences

The system SHALL present a skippable multi-step wizard that captures a visitor's job-feed preferences — role, seniority, work mode, region, and stack — without requiring an account.

The wizard presents preferences across steps: step one captures role and seniority; step two captures work mode, region, and stack. All fields are optional, and any field may be left empty. The wizard offers pickers built from the same controlled vocabularies the job search understands (categories, work modes, seniority levels, regions), so a captured preference is always a value the feed can filter on. The stack picker suggests skills from the live skills facet distribution (busiest first, with per-skill job counts) and filters them typo-tolerantly as the user types, so it only offers skills that exist in the catalogue.

#### Scenario: User completes the wizard with a full selection

- **WHEN** a visitor opens the wizard, selects a role, a seniority, a work mode, a region, and one or more stack items, and confirms
- **THEN** the feed reconfigures to those preferences and the wizard closes

#### Scenario: User confirms with a partial selection

- **WHEN** a visitor selects only a role and confirms without choosing seniority, work mode, region, or stack
- **THEN** the feed reconfigures using only the role and the wizard closes

#### Scenario: User picks a stack skill from the live suggestions

- **WHEN** a visitor types into the stack picker and selects a suggested skill
- **THEN** the skill is captured as a stack preference, drawn from the skills facet vocabulary, and rendered safely as text (no markup injection)

### Requirement: Wizard reconfigures the existing job feed through existing filters

The system SHALL translate the wizard's selections into the existing facet filter query and apply them through the existing filter store, so that the resulting feed, its facet counts, and subsequent re-editing behave identically to filters set through the standard filter UI.

The wizard MUST NOT introduce a parallel feed, ranking, or filter representation. Preferences left empty MUST NOT constrain the feed.

#### Scenario: Selections become standard filter query params

- **WHEN** the wizard applies a selection of role and work mode
- **THEN** the feed shows the same results as if those same facet values had been chosen in the standard filter UI

#### Scenario: Wizard result is editable through the standard filter UI

- **WHEN** a visitor completes the wizard and then opens the standard filter UI
- **THEN** the wizard's selections appear as the current filter state and can be changed or cleared through the normal controls

### Requirement: Onboarding is surfaced by a one-time banner

The system SHALL surface onboarding through a single unobtrusive banner shown above the feed. The banner is the only entry point to the wizard; once dismissed or completed it retires and is not shown again, and there is no persistent re-open control.

The banner SHALL appear only on the standalone jobs feed when onboarding has not been completed and no filters are already active, so it never interrupts a visitor who arrived via a shared filtered link. The banner does not block viewing the feed.

#### Scenario: Banner shown to a fresh visitor

- **WHEN** a visitor opens the standalone jobs feed with no active filters and has not completed onboarding
- **THEN** the banner is displayed above the feed and the feed remains browsable

#### Scenario: Banner hidden when arriving with active filters

- **WHEN** a visitor opens the jobs feed via a link that already carries filter params
- **THEN** the banner is not displayed

#### Scenario: Banner opens the wizard

- **WHEN** a visitor activates the banner's set-up action
- **THEN** the wizard opens

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

### Requirement: Onboarding can be skipped

The system SHALL allow a visitor to dismiss the banner or exit the wizard without making a selection, leaving the feed unchanged.

Dismissing the banner SHALL suppress it from reappearing, without altering the current feed.

#### Scenario: Visitor dismisses the banner

- **WHEN** a visitor dismisses the banner without opening the wizard
- **THEN** the banner is suppressed on subsequent visits and the feed is unchanged

#### Scenario: Visitor exits the wizard without confirming

- **WHEN** a visitor opens the wizard and closes it without confirming a selection
- **THEN** the feed is unchanged and the wizard closes

### Requirement: Narrow feed offers a relax-filter affordance

The system SHALL, when the configured preferences yield few or no matching jobs, display the honest result count and an action that relaxes the feed by removing the narrowest applied preference.

Relaxing SHALL remove one preference at a time in order of specificity — stack first, then region, then seniority — and reflect the change in the feed and its filter state. The system MUST NOT silently broaden the feed on its own.

#### Scenario: Zero results offers relax action

- **WHEN** the configured preferences match no open jobs
- **THEN** an empty-state message with the count and a "relax filter" action is shown

#### Scenario: Relaxing removes the narrowest preference first

- **WHEN** a visitor with role, region, and stack preferences activates the relax action
- **THEN** the stack preference is removed first and the feed and filter state update to reflect the broader query

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
