## ADDED Requirements

### Requirement: Hiding a job from the browse feed

The system SHALL let an authenticated user hide a job directly from a job card in
the browse feed. The card SHALL expose a hide control (an eye-off icon) revealed
on hover; activating it SHALL record the dismissed interaction via
`POST /api/v1/jobs/:slug/dismiss` and remove the card from the current feed. A
signed-out user who activates the control SHALL be routed to sign-in and the job
SHALL NOT be hidden. The hide control SHALL NOT navigate to the job detail (it is
an overlay sibling of the card link, like the save control).

#### Scenario: Authenticated user hides a job from the feed

- **WHEN** an authenticated user activates the hide control on a feed card
- **THEN** the system records the job as dismissed for that user
- **AND** the card is removed from the visible feed
- **AND** the browse view does not navigate to the job detail

#### Scenario: Signed-out user is routed to sign-in

- **WHEN** a signed-out visitor activates the hide control
- **THEN** the sign-in dialog opens
- **AND** no dismiss request is sent and the card stays in the feed

### Requirement: Hidden jobs are excluded from the browse feed

The system SHALL exclude a signed-in user's hidden (dismissed) jobs from the
browse feed by cross-referencing a client-side set of dismissed slugs, mirroring
the existing viewed/saved slug cross-reference. The set SHALL be loaded once for
a signed-in user via `GET /api/v1/me/tracking/dismissed`, which returns
`{"data": [slug, ...]}` scoped to the caller. Exclusion is client-side only; the
server search/Meili path SHALL be unchanged. For a signed-out user the set stays
empty and nothing is excluded.

#### Scenario: Dismissed jobs do not reappear in the feed

- **WHEN** a signed-in user who previously hid job A loads or re-renders the
  browse feed
- **THEN** job A is not shown among the feed cards

#### Scenario: Dismissed-slugs endpoint scopes to the caller

- **WHEN** an authenticated user requests `GET /api/v1/me/tracking/dismissed`
- **THEN** the system responds `200` with `{"data": [slug, ...]}` listing only
  that user's dismissed jobs' public slugs

#### Scenario: Signed-out feed excludes nothing

- **WHEN** a signed-out visitor loads the browse feed
- **THEN** no client-side dismissed exclusion is applied and every matching job
  is eligible to show

### Requirement: Undo a hide from the feed

When a user hides a job from the feed, the system SHALL surface a transient
"Job hidden — Undo" affordance. Activating Undo SHALL clear the job's dismissed
mark via `DELETE /api/v1/jobs/:slug/dismiss` and restore the job to the feed. The
affordance SHALL dismiss itself after a short timeout if not used. An undo request
for a job with no interaction row SHALL be treated as success (already not
hidden), never an error.

#### Scenario: Undo restores a just-hidden job

- **WHEN** a user hides a feed card and then activates Undo before it times out
- **THEN** the job's dismissed mark is cleared
- **AND** the job becomes eligible to show in the feed again

#### Scenario: Undo affordance auto-dismisses

- **WHEN** a user hides a feed card and does not activate Undo
- **THEN** the undo affordance disappears after its timeout without changing the
  hidden state

### Requirement: Reviewing and un-hiding jobs in Activity

The Activity surface SHALL provide a "Hidden" tab, at its own linkable route,
listing the signed-in user's hidden (dismissed) jobs most-recently-hidden first.
The listing SHALL be served by the tracking list under a `dismissed` filter that
selects interaction rows with `dismissed_at` set. Each listed job SHALL offer an
un-hide action that clears the dismissed mark via
`DELETE /api/v1/jobs/:slug/dismiss` and removes it from the Hidden list. When the
user has no hidden jobs the tab SHALL show an empty state.

#### Scenario: Hidden tab lists dismissed jobs

- **WHEN** an authenticated user with hidden jobs opens the Activity "Hidden" tab
- **THEN** the system lists those jobs, most-recently-hidden first

#### Scenario: Un-hiding removes a job from the Hidden list

- **WHEN** the user activates un-hide on a job in the Hidden tab
- **THEN** the job's dismissed mark is cleared
- **AND** the job is removed from the Hidden list

#### Scenario: Empty Hidden tab

- **WHEN** an authenticated user with no hidden jobs opens the "Hidden" tab
- **THEN** the system shows an empty state indicating nothing is hidden
