## ADDED Requirements

### Requirement: The entry point is visible to everyone

The job filter sidebar SHALL carry an AI-filter entry point above the All-filters button,
rendered for every visitor including signed-out ones. Hiding it from signed-out visitors
would hide the feature from exactly the people who have not yet been given a reason to
sign in.

The company filter summary SHALL NOT gain the entry point — it filters companies, not
jobs, and the interpretation's vocabulary is the job filter's.

#### Scenario: A signed-out visitor sees the entry point

- **WHEN** a signed-out visitor opens the job list
- **THEN** the AI-filter entry point is rendered in the filter sidebar

#### Scenario: The company filter is unaffected

- **WHEN** a visitor opens the company list
- **THEN** no AI-filter entry point is rendered

### Requirement: A signed-out click asks for sign-in

Activating the entry point without a session SHALL open the application's existing auth
dialog. No separate sign-in surface SHALL be introduced for this feature.

#### Scenario: Signed-out activation

- **WHEN** a signed-out visitor activates the AI-filter entry point
- **THEN** the existing auth dialog opens
- **AND** no interpretation request is sent

### Requirement: Two ways to seed a search

The dialog SHALL offer two seeds: free text, and the caller's saved profile.

The text seed SHALL show example descriptions, because a blank box does not tell someone
what kind of sentence works.

The profile seed SHALL render an empty state pointing at the profile page when the caller
has no saved profile, rather than an error.

#### Scenario: Text seed

- **WHEN** a signed-in caller types a description and submits it
- **THEN** an interpretation is requested from that text

#### Scenario: Profile seed

- **WHEN** a signed-in caller with a saved profile chooses the profile seed
- **THEN** an interpretation is requested from their profile without them typing anything

#### Scenario: Profile seed with no profile

- **WHEN** a signed-in caller with no saved profile chooses the profile seed
- **THEN** the dialog shows where to create a profile instead of an error

### Requirement: The preview shows what was understood before anything is applied

An interpretation SHALL be previewed, not applied on arrival. The preview SHALL show the
summary sentence, the resolved values as labelled chips grouped the way the sidebar groups
them, and — when anything was dropped — what was not recognised.

The filter state SHALL NOT change until the caller applies.

#### Scenario: Preview before apply

- **WHEN** an interpretation returns
- **THEN** the dialog shows its summary and resolved values
- **AND** the current filters are unchanged

#### Scenario: Unresolved values are surfaced

- **WHEN** an interpretation dropped a value it could not resolve
- **THEN** the preview names that value as not recognised

#### Scenario: Nothing resolved

- **WHEN** an interpretation resolved no values at all
- **THEN** the dialog says it could not turn that into filters and offers no apply action

### Requirement: Refining from the preview

The preview SHALL offer one field to add a constraint in words. Submitting it SHALL
request a fresh interpretation carrying the previewed result as context, and replace the
preview with the new result.

Closing the dialog SHALL discard the preview and any refinements; nothing is retained.

#### Scenario: Refine replaces the preview

- **WHEN** a caller adds a constraint from the preview
- **THEN** the preview is replaced by the newly interpreted result

#### Scenario: Dismissal discards

- **WHEN** a caller closes the dialog without applying
- **THEN** the filter state is unchanged and the preview is not retained

### Requirement: Applying replaces the filter state

Applying SHALL clear the current filters and write the interpreted result through the
filter store's published operations, so the URL, the result list and the applied-filter
chips follow exactly as they do for a hand-built filter.

Applying SHALL write the interpreted values and nothing else.

#### Scenario: Apply replaces rather than merges

- **WHEN** a caller with existing filters applies an interpretation
- **THEN** the previous filter values are gone and only the interpreted values remain

#### Scenario: Applied values behave like any other filter

- **WHEN** an interpretation has been applied
- **THEN** each value appears as a removable chip in the sidebar
- **AND** removing one narrows the filter exactly as removing a hand-picked value does

### Requirement: Saving an applied search uses the existing control

An applied interpretation SHALL be savable through the save-search-alert control already
present in the filter sidebar. No second save affordance SHALL be introduced for AI-built
filters — an AI-built filter is a filter.

#### Scenario: Save after apply

- **WHEN** a caller applies an interpretation
- **THEN** the existing save-search control saves that filter as a search alert
