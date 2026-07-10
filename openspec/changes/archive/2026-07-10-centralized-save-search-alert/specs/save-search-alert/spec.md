## ADDED Requirements

### Requirement: Save is the primary action; the Telegram alert follows

The system SHALL make saving the current filter query the primary, standalone action — it MUST work without any Telegram subscription — and SHALL offer the Telegram alert only once the search is saved. The flow is driven only by a filter query string, so the same behavior is reachable from any surface, and each step is idempotent.

Saving reuses the saved search whose canonical query matches the current query, or creates a new one with an auto-generated name; an "already saved" conflict is treated as success (no duplicate). Once a matching saved search exists, the system offers to turn on its Telegram alert, which ensures the account is linked and then creates the subscription; an "already subscribed" conflict is treated as success.

#### Scenario: Saving works without any subscription

- **WHEN** a signed-in user saves the current filters
- **THEN** a saved search is created (or an existing match reused) with no Telegram subscription required

#### Scenario: The alert is offered only after the search is saved

- **WHEN** the current query is not yet saved
- **THEN** the control presents the save action, not the Telegram alert; the alert offer appears only once the search is saved

#### Scenario: Saving the same query is idempotent

- **WHEN** the current query's canonical form matches an existing saved search and the user saves again
- **THEN** the existing saved search is reused and no duplicate is created

#### Scenario: Turning on the alert for a saved search is idempotent

- **WHEN** a user turns on the Telegram alert for a saved search that is already subscribed
- **THEN** the system treats it as success and leaves exactly one subscription

### Requirement: Auth gating with cross-redirect resume

The system SHALL require sign-in before saving, and SHALL resume the pending save after the user signs in — including across a full-page OAuth redirect.

When an unauthenticated user activates the save, the system opens the sign-in dialog and records the pending query in local storage. After sign-in completes — whether in-dialog or after an OAuth redirect back to the jobs page — the system replays the save for the recorded query and clears the record. A password sign-in that completes without navigating resumes it too.

#### Scenario: Signed-out user is prompted to sign in

- **WHEN** a signed-out user activates the save
- **THEN** the sign-in dialog opens and the intended query is recorded for resume

#### Scenario: Save resumes after an OAuth redirect

- **WHEN** a signed-out user activates the save, signs in via a provider that redirects the page away and back, and returns to the jobs page
- **THEN** the recorded pending query is saved and the record is cleared

#### Scenario: No stray resume without a pending record

- **WHEN** a signed-in user loads the jobs page with no pending query recorded
- **THEN** nothing is saved

### Requirement: Telegram linking is a guarded step within the flow

The system SHALL, when the user has not linked Telegram, drive the existing deep-link connection and re-check the link status before subscribing, without blocking the page.

If Telegram is not linked, the system opens the bot deep link and presents a "connecting" state with a manual re-check; once the link is confirmed it proceeds to subscribe. The flow MUST never leave the page unusable while waiting.

#### Scenario: Unlinked user connects then subscribes

- **WHEN** a signed-in but unlinked user activates the action and completes the Telegram connection
- **THEN** after the link is confirmed the subscription is created and the alert is reported as on

#### Scenario: Telegram disabled hides the alert offer

- **WHEN** the Telegram integration is disabled on the server
- **THEN** the alert offer is not shown, while saving a search remains available

### Requirement: The centralized control is surfaced everywhere it's needed

The system SHALL surface the shared save-and-alert control, using one implementation, from the filters sidebar, the post-onboarding banner, the saved-searches modal tab, and the account saved-searches page — each driven by a query string.

The sidebar renders the control beneath the "All filters" button, operating on the live filters. The post-onboarding banner offers it for the just-configured feed and is ephemeral (shown after completion and resumed after sign-in, not a persistent nag). The saved-searches modal tab renders it for its save/alert while keeping its list management (apply, rename, delete, update). The account page renders one per already-saved search (so it shows the alert offer / subscribed state).

#### Scenario: Sidebar control uses the current filters

- **WHEN** a user activates the sidebar control
- **THEN** it operates on the live filter query

#### Scenario: All surfaces share one behavior

- **WHEN** the same query is acted on from any surface
- **THEN** the resulting saved search and subscription are identical regardless of surface

### Requirement: Alerts are managed on one account page

The system SHALL manage saved searches and their Telegram alerts on a single account page, rather than a separate notifications page. The page presents the Telegram connection once, and each saved search carries its own alert toggle; a request for the former notifications page redirects to it.

#### Scenario: The former notifications page redirects

- **WHEN** a user opens the old notifications URL
- **THEN** they are redirected to the saved-searches page

#### Scenario: An alert is managed from the saved search it belongs to

- **WHEN** a user turns a saved search's Telegram alert on or off on the account page
- **THEN** the subscription for that saved search is created or removed

### Requirement: The flow degrades safely on error

The system SHALL keep the feed usable and surface a retryable error when any step of the flow fails, and MUST NOT break the page.

A network or server failure at any step (save, link, subscribe) leaves the feed intact and shows a retryable error state; it does not partially corrupt the user's saved searches or subscriptions beyond what already succeeded idempotently.

#### Scenario: A failed step is retryable

- **WHEN** a step in the flow fails due to a network or server error
- **THEN** an error state with a retry is shown and the jobs feed remains usable
