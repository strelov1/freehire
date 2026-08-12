## ADDED Requirements

### Requirement: Recording a delivery as an in-app notification

The system SHALL record one `user_notifications` row for every successful
delivery event in `internal/notify`, `internal/reminder`, and `internal/nudge`
— independent of which channel(s) carried it — carrying a kind, a
human-readable title and body, and, when the event concerns exactly one job,
that job's public slug. A recording failure SHALL NOT fail or roll back the
delivery it accompanies.

#### Scenario: A delivered subscription digest is recorded

- **WHEN** a saved-search subscription's digest is delivered over any channel
- **THEN** a `user_notifications` row is created with `kind=subscription_digest`, the digest's rendered title/body, and — only if the digest matched exactly one job — that job's public slug

#### Scenario: A delivered reminder is recorded

- **WHEN** a saved-job reminder is delivered over any channel
- **THEN** a `user_notifications` row is created with `kind=reminder` and the reminder's job's public slug (always present — a reminder always concerns one job)

#### Scenario: A delivered nudge is recorded

- **WHEN** an application nudge (follow-up, interview-prep, or job-closed) is delivered over any channel
- **THEN** a `user_notifications` row is created with the matching `kind` (`nudge_follow_up`/`nudge_interview_prep`/`nudge_job_closed`) and the nudge's job's public slug

#### Scenario: Recording failure does not affect delivery

- **WHEN** the `user_notifications` insert fails after a digest/reminder/nudge was successfully sent
- **THEN** the delivery is still marked notified/delivered as normal, and the failure is logged, not raised as a delivery error

### Requirement: Listing a user's notifications

The system SHALL let an authenticated user page through their own
`user_notifications`, newest first, via `GET /api/v1/me/notifications`, using
this codebase's standard offset/limit pagination (`limit`/`offset` query
params, bounded, `meta.total`/`meta.limit`/`meta.offset`), with an additional
`meta.unread_count` carrying the caller's total unread count. Cookie-only
(`RequireAuth`).

#### Scenario: Default page

- **WHEN** an authenticated user GETs `/api/v1/me/notifications` with no query params
- **THEN** the system returns their most recent notifications (default page size), with `meta.total`, `meta.unread_count`, and each row's read/unread state

#### Scenario: Pagination bounds

- **WHEN** a `limit` beyond the maximum or a negative `offset` is requested
- **THEN** the system clamps to the same bounds every other paginated `/me/*` endpoint enforces

### Requirement: Marking notifications read

The system SHALL let an authenticated user mark one notification read
(`POST /api/v1/me/notifications/:id/read`, idempotent, owner-scoped — another
user's id 404s) or mark all of their unread notifications read in one call
(`POST /api/v1/me/notifications/read-all`, returning the count marked).
Fetching the list SHALL NOT itself mark anything read.

#### Scenario: Mark one notification read

- **WHEN** an authenticated user POSTs `/api/v1/me/notifications/{id}/read` for a notification they own
- **THEN** its `read_at` is set (if not already) and the response confirms

#### Scenario: Marking an already-read notification is a no-op

- **WHEN** the same request is repeated
- **THEN** `read_at` is unchanged and the request still succeeds

#### Scenario: Cannot mark another user's notification

- **WHEN** a user POSTs `/api/v1/me/notifications/{id}/read` for a notification belonging to another user
- **THEN** the system returns 404 and marks nothing

#### Scenario: Mark all read

- **WHEN** an authenticated user POSTs `/api/v1/me/notifications/read-all`
- **THEN** every currently-unread notification of theirs is marked read, and the response reports how many were marked

#### Scenario: Listing does not mark read

- **WHEN** a user fetches `GET /api/v1/me/notifications`
- **THEN** no notification's `read_at` changes as a result

### Requirement: Notification center UI on web and mobile

Both the web app and the `freehire-mobile` app SHALL show a bell icon carrying
the caller's unread count, opening a list of notification cards (kind icon,
title/body text, relative timestamp, unread visual state). Tapping a card
whose kind carries a public slug SHALL navigate to that job (web and mobile),
except `nudge_follow_up`/`nudge_interview_prep` on web, which SHALL navigate to
the tracking board instead, matching the existing Telegram/email link target
for those two kinds. A card with no slug (a multi-job subscription digest)
SHALL have no navigation target.

#### Scenario: Unread badge reflects unread count

- **WHEN** the user has 3 unread notifications
- **THEN** the bell icon shows a badge with 3

#### Scenario: Tapping a job-bearing card navigates to the job

- **WHEN** the user taps a `reminder`, `nudge_job_closed`, or single-job `subscription_digest` card
- **THEN** the app navigates to that job's page

#### Scenario: Tapping a follow-up/interview-prep card on web opens the tracking board

- **WHEN** a web user taps a `nudge_follow_up` or `nudge_interview_prep` card
- **THEN** the app navigates to `/my/tracking`

#### Scenario: Tapping a follow-up/interview-prep card on mobile opens the job

- **WHEN** a mobile user taps a `nudge_follow_up` or `nudge_interview_prep` card
- **THEN** the app navigates to that job's page (no tracking-board screen exists in the mobile app)

#### Scenario: A multi-job digest card has no navigation target

- **WHEN** the user taps a `subscription_digest` card whose digest matched more than one job
- **THEN** nothing navigates
