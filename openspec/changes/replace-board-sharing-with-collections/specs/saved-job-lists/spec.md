## Purpose

Lets a signed-in user group specific jobs into named lists — independent of the
single-flag "save" — and optionally publish one read-only by slug so someone else
can see exactly those jobs without an account.

## ADDED Requirements

### Requirement: Create a job list

A signed-in user SHALL be able to create a named list with an optional
description. A list starts private (no public slug) and empty.

#### Scenario: Create a list
- **WHEN** an authenticated user sends `POST /api/v1/me/lists` with a valid `name` and an optional `description`
- **THEN** the system stores the list scoped to that user and responds `201` with `{"data": {id, name, description, public_slug, created_at, updated_at}}` where `public_slug` is empty

#### Scenario: Description defaults to empty
- **WHEN** an authenticated user creates a list without a `description`
- **THEN** the system stores an empty description and responds `201`

#### Scenario: Unauthenticated request is rejected
- **WHEN** a request without a valid session cookie hits any `/api/v1/me/lists` endpoint
- **THEN** the system responds `401` and stores nothing

### Requirement: Name validation

A job list name SHALL be trimmed and contain between 1 and 100 characters, and
SHALL be unique per user (case-sensitive after trim). A job list description,
when supplied, SHALL be trimmed and contain at most 2000 characters.

#### Scenario: Blank name rejected
- **WHEN** an authenticated user creates or renames a list with a name that is empty or only whitespace
- **THEN** the system responds `400` and stores nothing

#### Scenario: Duplicate name rejected
- **WHEN** an authenticated user creates or renames a list to a name they already use
- **THEN** the system responds `409` and does not create or modify a row

#### Scenario: Over-long description rejected
- **WHEN** the trimmed `description` exceeds 2000 characters
- **THEN** the system responds `400` and stores nothing

### Requirement: Per-user cap

The system SHALL allow at most 50 job lists per user.

#### Scenario: Cap exceeded on create
- **WHEN** an authenticated user who already has 50 job lists sends a create request
- **THEN** the system responds `409` and stores nothing

### Requirement: Per-list job cap

The system SHALL allow at most 200 jobs in a single list. Re-adding a job the
list already contains SHALL NOT count against this cap (it changes nothing,
per the idempotency requirement above). This bounds the cost of the public,
unauthenticated read of a shared list (which renders every job it contains)
to a predictable size per request.

#### Scenario: Cap exceeded on add
- **WHEN** an authenticated user adds a new job to a list that already contains 200 jobs
- **THEN** the system responds `409` and the list is unchanged

#### Scenario: Re-adding an existing member is exempt from the cap
- **WHEN** an authenticated user adds a job that is already in a list holding 200 jobs
- **THEN** the system responds successfully (the idempotent no-op), not `409`

### Requirement: List and manage own job lists

A signed-in user SHALL be able to list their own job lists, most recently
updated first, rename a list, update its description, and delete a list.
Deleting a list removes its membership rows but SHALL NOT affect the jobs
themselves or the user's separate "save" flags.

#### Scenario: List own lists
- **WHEN** an authenticated user sends `GET /api/v1/me/lists`
- **THEN** the system responds `200` with `{"data": [...]}` containing only that user's job lists ordered by `updated_at` descending, each including its job count and shared state (`public_slug`, empty when private)

#### Scenario: Rename
- **WHEN** an authenticated user sends `PATCH /api/v1/me/lists/:id` with a new `name`
- **THEN** the system replaces the stored name (subject to name validation), bumps `updated_at`, and responds `200`

#### Scenario: Update description
- **WHEN** an authenticated user sends `PATCH /api/v1/me/lists/:id` with a new `description`
- **THEN** the system replaces the stored description and responds `200`

#### Scenario: Delete own list
- **WHEN** an authenticated user sends `DELETE /api/v1/me/lists/:id` for a list they own
- **THEN** the system removes the list and its membership rows, leaves the referenced jobs and the user's `save` flags untouched, and responds `204`

### Requirement: User-scoped access

Every job-list operation SHALL be scoped to the calling user; one user MUST
NOT be able to read, modify, or delete another user's job list.

#### Scenario: Cannot touch another user's list
- **WHEN** an authenticated user sends `PATCH`, `DELETE`, or a membership change for a list id owned by a different user
- **THEN** the system responds `404` and the target list is unchanged

### Requirement: Add and remove jobs in a list

A signed-in user SHALL be able to add a specific job to one of their own lists
and remove one. Jobs are addressed by their public slug, the same wire
identifier used everywhere else a job is referenced (save/unsave, tracking,
...) — never by an internal numeric id. A job MAY belong to any number of
lists (zero or more), independently of whether it is "saved" via the separate
save flag. Adding a job already in the list SHALL be idempotent; removing a
job not in the list, or a slug that resolves to no job at all, SHALL be
idempotent.

#### Scenario: Add a job to a list
- **WHEN** an authenticated user sends `POST /api/v1/me/lists/:id/jobs` with a `job_slug` for a list they own
- **THEN** the system adds the job to the list and responds `200` (or `204`) with the list unaffected in name/description

#### Scenario: Adding an already-present job is a no-op
- **WHEN** an authenticated user adds a `job_slug` that is already in the list
- **THEN** the system makes no change beyond ensuring membership and responds successfully without creating a duplicate entry

#### Scenario: Remove a job from a list
- **WHEN** an authenticated user sends `DELETE /api/v1/me/lists/:id/jobs/:job_slug` for a list they own
- **THEN** the system removes the job from the list and responds `204`

#### Scenario: Removing an absent job is a no-op
- **WHEN** an authenticated user removes a `job_slug` that is not in the list
- **THEN** the system responds `204` without error

#### Scenario: Unknown job slug rejected on add
- **WHEN** an authenticated user adds a `job_slug` that does not exist
- **THEN** the system responds `404` and the list is unchanged

#### Scenario: Unknown job slug is a no-op on remove
- **WHEN** an authenticated user removes a `job_slug` that does not exist
- **THEN** the system responds `204` without error, since the goal state (the job is not in the list) already holds

### Requirement: Share a job list as a public, read-only page

A signed-in user SHALL be able to make one of their own job lists public by
minting a stable public slug for it. Sharing an already-shared list is
idempotent for the slug (the existing slug is kept).

#### Scenario: Share a private list
- **WHEN** an authenticated user sends `POST /api/v1/me/lists/:id/share` for a list they own that has no public slug
- **THEN** the system generates a public slug from the list's name plus a short random suffix, stores it, and responds `200` with `{"data": {id, name, description, public_slug, ...}}`

#### Scenario: Re-share keeps the slug
- **WHEN** an authenticated user sends the share request for a list that already has a public slug
- **THEN** the system keeps the existing slug and responds `200`

#### Scenario: Cannot share another user's list
- **WHEN** an authenticated user sends the share request for a list id owned by a different user
- **THEN** the system responds `404` and mints no slug

#### Scenario: Unauthenticated share rejected
- **WHEN** a request without a valid session cookie hits the share endpoint
- **THEN** the system responds `401` and changes nothing

### Requirement: Unshare a job list

A signed-in user SHALL be able to make one of their own shared job lists
private again, clearing its public slug. Unsharing invalidates the previously
issued public link; a subsequent share mints a new slug rather than reviving
the old one.

#### Scenario: Unshare a shared list
- **WHEN** an authenticated user sends `DELETE /api/v1/me/lists/:id/share` for a list they own that has a public slug
- **THEN** the system clears the public slug (the list becomes unreachable by that slug) and responds `204`

#### Scenario: Unshare a list that is not shared
- **WHEN** an authenticated user unshares a list they own that has no public slug
- **THEN** the system responds `204` (idempotent no-op)

#### Scenario: Cannot unshare another user's list
- **WHEN** an authenticated user unshares a list id owned by a different user
- **THEN** the system responds `404` and the target list is unchanged

### Requirement: Public read of a shared job list by slug

The system SHALL serve a shared job list to anyone by its public slug without
authentication, exposing the list's name, description, and its jobs. The
owner's identity (user id, email) MUST NOT be exposed. A job that has since
closed or expired SHALL still appear in the list, carrying its current status,
rather than being silently dropped. A slug that does not exist or belongs to a
list that is not currently shared SHALL be a 404.

#### Scenario: Read a shared list
- **WHEN** any client sends `GET /api/v1/lists/:slug` for a currently shared list
- **THEN** the system responds `200` with `{"data": {name, description, jobs: [...]}}` and no owner-identifying fields

#### Scenario: Closed job stays in the shared list
- **WHEN** a shared list contains a job that has since closed
- **THEN** the response still includes that job, reflecting its closed status, rather than omitting it

#### Scenario: Unknown or unshared slug
- **WHEN** the slug does not exist, or names a list whose public slug has been cleared
- **THEN** the system responds `404`

### Requirement: Public job list page in the web app

The web app SHALL expose a public, unauthenticated route `/l/:slug` that loads
a shared job list and renders its name, description, and jobs. An unknown slug
SHALL render a not-found state.

#### Scenario: Open a shared list link
- **WHEN** a visitor opens `/l/:slug` for a shared list
- **THEN** the page shows the list name, its description, and the jobs it contains (including any that have since closed, marked as such)

#### Scenario: Open an unknown list link
- **WHEN** a visitor opens `/l/:slug` for a slug that is not shared
- **THEN** the page renders a not-found state rather than an empty jobs list

### Requirement: Job lists section in the account area

The web app SHALL expose a dedicated account section that lists the
signed-in user's job lists and lets them manage each one: create a list,
rename it, edit its description, share it as a public page, unshare it, copy
its public `/l/:slug` link when shared, and delete it. Adding or removing a
specific job is done from the job card's "Add to list" control (see the next
requirement), not from this section — this section manages the lists
themselves, and shows each one's job count rather than its contents. An
anonymous visitor SHALL be prompted to sign in rather than shown a list.

#### Scenario: List and manage from the account section
- **WHEN** a signed-in user opens the job lists account section
- **THEN** the page lists their job lists, each showing its job count and whether it is shared, with actions to rename, edit description, share, unshare, delete, and (when shared) copy its public link

#### Scenario: Anonymous access to the section
- **WHEN** an anonymous (signed-out) visitor opens the job lists account section
- **THEN** the page prompts sign-in instead of listing job lists

### Requirement: Add-to-list affordance on the job card

The web app SHALL offer an "Add to list" control on a job's card and detail
page, independent of the existing "Save" star, letting a signed-in user add or
remove that job from any of their existing lists or create a new list
containing it. An anonymous user SHALL be prompted to sign in instead.

#### Scenario: Add a job to an existing list from its card
- **WHEN** a signed-in user opens the "Add to list" control on a job and selects an existing list
- **THEN** the job is added to that list and the control reflects the updated membership

#### Scenario: The control shows current membership when opened
- **WHEN** a signed-in user opens the "Add to list" control on a job that already belongs to one of their lists
- **THEN** that list is shown as already containing the job, distinguishing it from lists the job is not in

#### Scenario: Remove a job from a list via the same control
- **WHEN** a signed-in user opens the "Add to list" control on a job that belongs to one of their lists and deselects that list
- **THEN** the job is removed from that list and the control reflects the updated membership

#### Scenario: Create a new list from the job card
- **WHEN** a signed-in user opens the "Add to list" control and creates a new list from it
- **THEN** the system creates the list and adds the current job to it in one action

#### Scenario: Anonymous prompt
- **WHEN** an anonymous (signed-out) user opens the "Add to list" control
- **THEN** the control shows a "sign in" affordance instead of a list of lists
