## ADDED Requirements

### Requirement: Activity section

The signed-in account area SHALL provide an **Activity** section served at `/my/activity` presenting three sub-tabs — **Saved**, **History**, and **Matches** — with **Saved** as the index tab (`/my/activity`). The Saved tab SHALL list the caller's saved-but-not-applied jobs using the existing `saved` interaction filter; the History tab SHALL list the caller's viewed jobs; the Matches tab SHALL list the caller's AI-fit analyses. The History and Matches views SHALL be the same ones previously shown under Tracking, relocated unchanged. Each sub-tab SHALL be its own URL (`/my/activity`, `/my/activity/history`, `/my/activity/matches`), reload-safe and bookmarkable.

#### Scenario: Opening Activity shows the Saved list

- **WHEN** a signed-in user opens `/my/activity`
- **THEN** the Saved tab is active and the caller's saved-but-not-applied jobs are listed

#### Scenario: History and Matches reachable under Activity

- **WHEN** a signed-in user selects the History or Matches sub-tab
- **THEN** the app navigates to `/my/activity/history` or `/my/activity/matches` and renders the same view formerly served under Tracking

#### Scenario: Empty Saved list

- **WHEN** a signed-in user with no saved jobs opens `/my/activity`
- **THEN** a friendly empty state is shown instead of a job list

### Requirement: Board shows only active applications

The Tracking Board SHALL present tracked jobs only in an active application state, across the columns **Applied**, **Interview**, **Offer**, and **Closed**, and SHALL NOT present a Saved column. A tracked job that is saved but has no application (no `applied_at` and no `stage`) SHALL NOT appear on the Board; such a job appears in the Activity → Saved list instead. Clearing a job's stage from the Board drawer (the "No stage" choice) SHALL remove the card from the Board while keeping the job's saved mark (it becomes saved-only), and SHALL NOT delete the interaction row or its view history.

#### Scenario: Saved-only job is not on the Board

- **WHEN** a signed-in user has a job that is saved but never applied to and carries no stage
- **THEN** the job does not appear on the Board and is listed under Activity → Saved

#### Scenario: Clearing a stage takes the card off the Board

- **WHEN** a signed-in user chooses "No stage" for a card in the Board drawer
- **THEN** the card is removed from the Board, the job keeps its saved mark, and it appears under Activity → Saved

#### Scenario: Board has no Saved column

- **WHEN** a signed-in user opens the Tracking Board
- **THEN** the columns are Applied, Interview, Offer, and Closed, with no Saved column

## MODIFIED Requirements

### Requirement: Tracking section renamed with URL redirects

The frontend personal-jobs section SHALL be presented as **Tracking** and served under `/my/tracking/*` with **Board** and **Pipeline** sub-tabs. History and AI-fit (Matches) SHALL NOT be Tracking sub-tabs; they are served under the Activity section (`/my/activity`). Requests to the previous `/my/jobs/*` URLs MUST redirect (HTTP 308) to the corresponding `/my/tracking/*` path so existing bookmarks and inbound links keep working. The retired `/my/tracking/history` and `/my/tracking/analyses` URLs are `noindex`, auth-gated personal pages and MAY 404 without a redirect.

#### Scenario: Old URL redirects to the new section

- **WHEN** a user opens `/my/jobs/pipeline`
- **THEN** the app redirects to `/my/tracking/pipeline`

#### Scenario: Section labelled Tracking with Board and Pipeline tabs

- **WHEN** a signed-in user opens the tracking section
- **THEN** the navigation and heading read "Tracking", with tabs for Board and Pipeline only
