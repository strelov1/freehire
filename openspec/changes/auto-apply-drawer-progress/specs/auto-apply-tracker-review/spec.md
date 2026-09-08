## ADDED Requirements

### Requirement: A tailoring or approved attempt is visible in the tracker drawer

The system SHALL render a status indicator in the tracker drawer for an attempt whose status is
`tailoring` or `approved`, so the candidate can tell auto-apply is actively working on the job
even before there is anything to decide. Neither indicator SHALL offer an Approve/Decline action,
since neither status has a pending decision.

#### Scenario: An attempt is still being tailored

- **WHEN** the candidate views an attempt whose status is `tailoring`
- **THEN** the drawer shows a status indicator that a tailored CV is being prepared, with no
  action to take

#### Scenario: An attempt has been approved

- **WHEN** the candidate views an attempt whose status is `approved`
- **THEN** the drawer shows a status indicator that the attempt is queued for automatic
  submission, with no Approve/Decline action

### Requirement: An approved attempt exposes the tailored CV for viewing

When an attempt's status is `approved`, the system SHALL let the candidate open the tailored CV
the attempt will submit, the same way a `pending_review` attempt already does.

#### Scenario: Viewing the tailored CV after approval

- **WHEN** the candidate views an attempt whose status is `approved`
- **THEN** the drawer offers a link to the tailored CV, and following it opens the same tailored
  CV a `pending_review` attempt for the same job would show

### Requirement: Outcome notifications deep-link to the specific application

When a submitted, blocked, or failed auto-apply outcome notification is about exactly one
application, the system SHALL link it to that application's tracker drawer rather than the
general tracker board, on every channel that renders a single-application notification (email,
Telegram, in-app). A notification batch covering more than one application SHALL continue to link
to the general tracker board, since no single application can be deep-linked to.

#### Scenario: A single-application outcome notification links to that application

- **WHEN** a submitted, blocked, or failed outcome notification is delivered for exactly one
  application
- **THEN** the notification's link opens that application's tracker drawer directly

#### Scenario: A batched outcome notification links to the general board

- **WHEN** a submitted, blocked, or failed outcome notification is delivered as a batch covering
  more than one application
- **THEN** the notification's link opens the general tracker board

### Requirement: The board can be filtered to only applications needing auto-apply review

The tracker board SHALL offer a toggle that narrows every column to only the applications whose
auto-apply status already earns the existing review badge (`pending_review` or `blocked`). The
toggle SHALL combine with the existing text search rather than replace it: with both active, only
applications matching both conditions SHALL be shown.

#### Scenario: Toggling the filter narrows the board

- **WHEN** the candidate enables the "needs attention" toggle
- **THEN** every column shows only applications whose auto-apply status is `pending_review` or
  `blocked`

#### Scenario: The filter combines with search

- **WHEN** the candidate has both a search query and the "needs attention" toggle active
- **THEN** the board shows only applications that match the search query AND need auto-apply
  review

#### Scenario: Turning the filter off restores the board

- **WHEN** the candidate disables the "needs attention" toggle
- **THEN** the board returns to showing every application matching the search query alone (or
  everything, if the search query is blank)

### Requirement: An application needing review is visually marked on the board card

In addition to its existing text label, a board card whose auto-apply status is `pending_review`
or `blocked` SHALL display a distinct visual marker (a colored dot) so the card is identifiable
without reading its text.

#### Scenario: A card needing review shows the marker

- **WHEN** a board card's auto-apply status is `pending_review` or `blocked`
- **THEN** the card renders both its existing "Review" label and the colored dot marker, together

#### Scenario: A card not needing review shows neither

- **WHEN** a board card's auto-apply status is anything other than `pending_review` or `blocked`
  (including no auto-apply attempt at all)
- **THEN** the card renders neither the label nor the dot marker
