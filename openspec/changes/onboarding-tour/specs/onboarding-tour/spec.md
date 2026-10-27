## ADDED Requirements

### Requirement: Tour is shown once and only asks what is missing

The system SHALL route a signed-in account whose onboarding is not complete to the tour, and SHALL mark the account's onboarding complete as soon as the tour is shown, so the account is routed there at most once. The mark is best-effort: if that write fails, the account is offered the tour once more on a later visit rather than being held on it. A reload of the tour after it was shown leaves it, like any other way out. The tour SHALL skip a data step whose answer the account already has — the CV step when a CV is stored, the feed step when the search profile has a specialization and a skill, the alert step when the profile alert is subscribed on any channel, the challenge step when a challenge is stored. The introductory steps (AI assistant, CV tailoring, extension) SHALL be shown whenever at least one data step is. An account with no data step left SHALL be marked complete without seeing a screen.

#### Scenario: New account sees the whole tour
- **WHEN** a newly registered account with no CV, profile, alert or challenge lands on any page
- **THEN** it is taken to the tour, which shows all seven steps

#### Scenario: Existing account missing only the alert
- **WHEN** an account that has a CV, a search profile and a stored challenge, but no subscribed profile alert, is routed to the tour
- **THEN** the tour shows the alert step and the three introductory steps, and nothing else

#### Scenario: Existing account missing nothing
- **WHEN** an account that has every data step answered is routed to the tour
- **THEN** its onboarding is marked complete and it returns to where it was headed without seeing a screen

#### Scenario: Leaving early does not bring it back
- **WHEN** an account is shown the tour and navigates away from its first step
- **THEN** a later visit does not route the account to the tour again

### Requirement: Stored answers pre-fill the tour

Every data step the tour shows SHALL open with what the account already has stored: the feed step with the search profile's specialization, level, skills and location; the challenge step with the stored challenges and note.

#### Scenario: Partly filled profile
- **WHEN** an account with a stored specialization but no skills is shown the feed step
- **THEN** its specialization is already selected

### Requirement: CV step pre-fills feed preferences

The CV step SHALL accept a PDF and, once parsed, SHALL pre-fill the feed step's specialization, level and skills, and the feed step SHALL say the values came from the CV.

#### Scenario: Uploading a CV
- **WHEN** a visitor uploads a readable PDF CV on the first step
- **THEN** the feed step opens with specialization, level and skills taken from it and a note saying so

### Requirement: Feed step saves the search profile and shows its result

The feed step SHALL collect specialization, level, skills and location on one screen and SHALL save them to the account's search profile. It SHALL show the number of open jobs matching the current picks, updated as they change. The specialization picker SHALL show technical categories before the others, each group ordered by open-job count, and SHALL match a typed query tolerantly of typos.

#### Scenario: Live match count
- **WHEN** a visitor picks a specialization on the feed step
- **THEN** the shown count of matching open jobs changes to reflect it

#### Scenario: Typo in a specialization search
- **WHEN** a visitor types "sofware" into the specialization search
- **THEN** Software Engineering is offered

#### Scenario: Technical categories lead
- **WHEN** the feed step opens with no search typed
- **THEN** the first specializations shown are technical categories

### Requirement: Alert channels step

The alert step SHALL offer the profile-derived alert over email (a switch that creates the alert and subscribes it), Telegram (connect the bot, then subscribe) and push (a link to the mobile app, the only push receiver), and SHALL show a preview of the alert email.

#### Scenario: Turning on email
- **WHEN** a visitor turns the email switch on
- **THEN** a profile-derived saved search exists and is subscribed over email

### Requirement: Challenge answers choose what helps

The challenge step SHALL accept several answers from the challenge vocabulary, except that "just starting" excludes every other answer. It SHALL save them to the candidate survey, with a free-text note only when "other" is among them, and SHALL show, for the picked answers together, the features that address them, each feature once.

#### Scenario: Two answers
- **WHEN** a visitor picks "Getting visa sponsorship" and "Language requirements"
- **THEN** the panel shows the visa-sponsorship filter, the visa-sponsor companies and the language filter

#### Scenario: Just starting replaces the rest
- **WHEN** a visitor who picked two answers then picks "Nothing yet — I'm just starting"
- **THEN** only "just starting" remains picked

### Requirement: Pro steps describe the paid features

The tailoring and extension steps SHALL explain the feature, link to its feature page, mark what is Pro — CV tailoring, and form filling in the extension — and link to the plans page. The extension step SHALL say the extension itself, search, alerts and the AI connector stay free.

#### Scenario: Extension step
- **WHEN** a visitor reaches the extension step
- **THEN** form filling is marked Pro and the step says the extension itself is free

