## ADDED Requirements

### Requirement: The site asks the visitor to star the repository

The site SHALL show a dismissible toast inviting the visitor to star the freehire
repository on GitHub, stating that the project is open source.

The toast SHALL float above the page rather than occupy the document flow, so that it
never shifts content a visitor is already reading.

#### Scenario: The invitation is shown

- **WHEN** the toast is shown
- **THEN** it names freehire as open source, and offers a link to the repository on
  GitHub that opens in a new tab

#### Scenario: The star count is shown when known

- **WHEN** the toast is shown and a star count is available
- **THEN** the count is rendered next to the link in compact form

#### Scenario: The star count is unavailable

- **WHEN** the toast is shown and no star count is available, because the cache is empty
  and the GitHub API did not answer
- **THEN** the toast is still shown, with a working link and no number

### Requirement: The ask queues behind the Product Hunt strip

The toast SHALL NOT be shown while the Product Hunt launch strip is still asking for
support, so that the visitor never faces two pleas at once.

#### Scenario: The launch strip is still asking

- **WHEN** the launch day has not passed and the visitor has not closed the Product Hunt
  strip
- **THEN** the toast is not shown

#### Scenario: The visitor closed the launch strip

- **WHEN** the visitor has closed the Product Hunt strip and the launch day has not
  passed
- **THEN** the toast is shown

#### Scenario: The launch day has passed

- **WHEN** the launch day has passed, so the Product Hunt strip no longer renders and
  can no longer be closed
- **THEN** the toast is shown, whether or not the strip was ever closed

### Requirement: The ask yields to obligations and to reversible actions

The toast SHALL NOT be shown while the cookie-consent banner awaits a decision, and
SHALL be layered below the hide-a-job Undo toast.

#### Scenario: Consent is undecided

- **WHEN** the cookie-consent banner is visible
- **THEN** the toast is not shown

#### Scenario: Consent is settled

- **WHEN** the visitor accepts or rejects cookies and the consent banner disappears
- **THEN** the toast appears without a reload, provided its other conditions hold

#### Scenario: An Undo is on screen

- **WHEN** the hide-a-job Undo toast and the support toast are on screen together
- **THEN** the Undo toast is drawn above the support toast and stays clickable

### Requirement: The ask is made once

Once the visitor has answered the ask — by closing the toast or by following the link —
the toast SHALL NOT be shown again.

#### Scenario: The visitor closes the toast

- **WHEN** the visitor closes the toast
- **THEN** it is not shown again on any later page or visit

#### Scenario: The visitor follows the link

- **WHEN** the visitor follows the link to GitHub
- **THEN** the toast is treated as answered and is not shown again

#### Scenario: Local storage is unavailable

- **WHEN** local storage cannot be read or written, as in a private-mode or blocked
  origin
- **THEN** the toast neither throws nor blocks the page; an unreadable store reads as
  "not answered", and an unwritable one limits the dismissal to the current page

### Requirement: The ask is absent where it would be redundant

The toast SHALL NOT be shown on the open-source page, which already makes the same case
at length.

#### Scenario: Visiting the open-source page

- **WHEN** the visitor is on `/open`
- **THEN** the toast is not shown, and no dismissal is recorded
