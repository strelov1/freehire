## ADDED Requirements

### Requirement: Member offers to refer into a company

A signed-in member SHALL be able to offer to refer into exactly one company per
offer by selecting a company and uploading a CV as proof of employment. The
offer enters moderation with status `pending`. The system SHALL enforce at most
one offer per `(member, company)` pair.

#### Scenario: Submitting a referral offer

- **WHEN** a signed-in member selects a company and uploads a proof CV
- **THEN** a `referral_offers` row is created with status `pending`, the proof
  CV stored via the résumé storage path, and the member is shown the pending
  state

#### Scenario: Duplicate offer for the same company

- **WHEN** a member submits an offer for a company they already have an offer for
- **THEN** the system rejects the duplicate and returns the existing offer
  rather than creating a second row

#### Scenario: Offer requires a proof CV

- **WHEN** a member submits an offer without a proof CV
- **THEN** the request is rejected with a validation error and no offer is created

### Requirement: Moderator reviews referral offers

A moderator SHALL be able to list `pending` referral offers, view each offer's
proof CV, and approve or reject it. Approval sets status `approved`; rejection
sets status `rejected`. Both record the deciding moderator and decision time.
Only `approved` offers make a company eligible for referral requests.

#### Scenario: Approving an offer

- **WHEN** a moderator approves a pending offer
- **THEN** the offer's status becomes `approved` with `decided_by` and
  `decided_at` recorded, and the company becomes referral-eligible

#### Scenario: Rejecting an offer

- **WHEN** a moderator rejects a pending offer
- **THEN** the offer's status becomes `rejected` and the company gains no
  referral eligibility from it

#### Scenario: Non-moderator cannot review

- **WHEN** a non-moderator calls the offer review endpoint
- **THEN** the request is denied with an authorization error

### Requirement: Referral availability signal on company and job

The company read shape and each `jobview` for that company SHALL expose whether
the company has at least one `approved` referral offer, so the frontend can show
an "ask for a referral" affordance only when a referral is available.

#### Scenario: Company has an approved referrer

- **WHEN** a company has at least one approved referral offer
- **THEN** the company and its jobs report referral availability as true

#### Scenario: Company has no approved referrer

- **WHEN** a company has no approved referral offer (or only pending/rejected)
- **THEN** the company and its jobs report referral availability as false

### Requirement: Seeker requests a referral

A signed-in seeker SHALL be able to request a referral into a referral-eligible
company. The request SHALL specify which CV to attach — the seeker's stored
original résumé or a tailored CV from the builder — a contact consisting of a
Telegram handle and/or an email (at least one required), and an optional note.
The source `job_id` MAY be recorded as context. The request enters status
`sent`.

#### Scenario: Submitting a referral request with an original CV

- **WHEN** a seeker requests a referral choosing their original résumé and
  provides at least one contact
- **THEN** a `referral_requests` row is created with status `sent`,
  `cv_kind = original`, the contact, and any source `job_id`

#### Scenario: Submitting a referral request with a tailored CV

- **WHEN** a seeker requests a referral choosing a tailored CV they own
- **THEN** the request records `cv_kind = built` and the referenced `cv_id`

#### Scenario: Request requires a contact

- **WHEN** a seeker submits a request with neither a Telegram handle nor an email
- **THEN** the request is rejected with a validation error and no request is created

#### Scenario: Requesting from a non-eligible company

- **WHEN** a seeker requests a referral into a company with no approved referrer
- **THEN** the request is rejected

### Requirement: One active request per seeker and company

The system SHALL allow at most one active (`sent`) referral request per
`(seeker, company)` pair, and SHALL apply a soft per-day cap on the number of
referral requests a seeker can create.

#### Scenario: Duplicate active request

- **WHEN** a seeker submits a second request for a company where they already
  have a `sent` request
- **THEN** the system rejects the duplicate

#### Scenario: Re-requesting after resolution

- **WHEN** a seeker's prior request for a company was marked `contacted` or
  `declined`
- **THEN** the seeker MAY submit a new request for that company

#### Scenario: Exceeding the daily cap

- **WHEN** a seeker exceeds the per-day request cap
- **THEN** further requests that day are rejected

### Requirement: Referrers are notified of new requests

When a referral request is created, the system SHALL notify every approved
referrer of that company through their own channel: email always, plus Telegram
if the referrer has linked it. Notifications SHALL NOT reveal a referrer's
identity to the seeker. A referrer with no reachable channel still sees the
request in their cabinet.

#### Scenario: Pinging approved referrers

- **WHEN** a seeker creates a referral request for a company with two approved
  referrers
- **THEN** both referrers receive a notification pointing to the request in
  their cabinet

#### Scenario: Telegram-linked referrer

- **WHEN** an approved referrer has linked Telegram
- **THEN** they receive the notification on Telegram in addition to email

### Requirement: Referrer manages incoming requests

An approved referrer SHALL see the incoming referral requests for the companies
they refer into, including the seeker's contact, chosen CV, note, and source
job. The referrer SHALL be able to view the attached CV only within their
authorized cabinet, and SHALL be able to mark a request `contacted` or
`declined`. Marking records which referrer acted and when.

#### Scenario: Viewing an incoming request

- **WHEN** an approved referrer opens their incoming requests
- **THEN** they see each `sent` request's contact, CV, note, and source job for
  their companies

#### Scenario: CV is only viewable when authorized

- **WHEN** anyone who is not an approved referrer of the request's company tries
  to view the attached CV
- **THEN** access is denied

#### Scenario: Marking a request contacted

- **WHEN** an approved referrer marks a request `contacted`
- **THEN** the request status becomes `contacted` with the acting referrer and
  time recorded, and it leaves the pool of `sent` requests

#### Scenario: Declining a request

- **WHEN** an approved referrer marks a request `declined`
- **THEN** the request status becomes `declined` with the acting referrer recorded

### Requirement: Seeker tracks their requests

A seeker SHALL see their referral requests in their cabinet, each showing the
target company, the CV they attached, and the current status
(`sent`, `contacted`, or `declined`).

#### Scenario: Listing own requests

- **WHEN** a seeker opens their referral requests
- **THEN** they see each request's company, attached CV, and current status

#### Scenario: Status reflects referrer action

- **WHEN** a referrer marks the seeker's request `contacted` or `declined`
- **THEN** the seeker's cabinet reflects the new status
