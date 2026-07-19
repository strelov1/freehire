## ADDED Requirements

### Requirement: Credits page shows the current balance

The system SHALL provide an authenticated page at `/my/credits`, titled "Credits", that displays
the caller's current AI-credits balance — the credits remaining in the current period and the
date the monthly grant resets — read from the existing balance endpoint without consuming any
credits.

#### Scenario: Balance is displayed

- **WHEN** a signed-in user opens `/my/credits`
- **THEN** the page shows the credits remaining this month and the reset date, and consumes no credits

#### Scenario: Anonymous access is gated

- **WHEN** an unauthenticated visitor navigates to `/my/credits`
- **THEN** the account shell's auth gate applies, as for every other `my/*` page

### Requirement: Credit transaction history endpoint

The system SHALL expose `GET /api/v1/me/credits/history`, authenticated by session cookie or API
key, returning the caller's own credit-ledger entries newest first. Each entry SHALL carry its
kind (monthly grant, match debit, tailor debit, or contribution reward), its signed delta, and
its timestamp. The endpoint SHALL be scoped to the caller and never reveal another user's ledger.

#### Scenario: History is returned newest first

- **WHEN** an authenticated user requests their credit history
- **THEN** the response lists only that user's ledger entries, ordered newest first, each with kind, signed delta, and timestamp

#### Scenario: Anonymous request is rejected

- **WHEN** an unauthenticated caller requests the credit history
- **THEN** the system responds 401 and returns no ledger data

### Requirement: History entries are human-labelled

The history SHALL render each entry with a human-readable label: a monthly grant, a contribution
reward, or a metered action naming its subject — a match debit resolves its ref to the analysed
job's title or slug, and a tailor debit resolves its ref to the tailored CV. When a debit's
referenced subject no longer exists, the entry SHALL still render with a stable fallback label
and its amount.

#### Scenario: Match debit names the job

- **WHEN** the history includes a match debit whose ref is an existing job
- **THEN** the entry is labelled with that job's title (or slug) and shows the −1 amount

#### Scenario: Grant and reward are labelled without a subject

- **WHEN** the history includes a monthly grant and a contribution reward
- **THEN** they render as "Monthly grant" (+20) and "Board contribution" (+5) respectively

#### Scenario: Missing subject falls back gracefully

- **WHEN** a debit references a job or CV that has since been deleted
- **THEN** the entry still renders with a generic label for its feature and its amount

### Requirement: Balance widget is consolidated onto the Credits page

The inline AI-credits balance widget SHALL NOT be shown on the Activity → Matches tab or on the
Profile page; the balance is surfaced on the dedicated Credits page (and via the balance endpoint).

#### Scenario: Matches tab no longer shows the widget

- **WHEN** a signed-in user opens the Activity → Matches tab
- **THEN** no inline credits-balance widget is rendered on that tab

#### Scenario: Profile page no longer shows the widget

- **WHEN** a signed-in user opens the Profile page
- **THEN** no inline credits-balance widget is rendered on that page
