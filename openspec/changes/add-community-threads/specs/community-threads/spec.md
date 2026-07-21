## ADDED Requirements

### Requirement: Create a discussion thread on a subject

A signed-in user SHALL create a discussion thread attached to a subject. A
subject is identified by a `subject_type` and the subject's public slug. At
launch the supported `subject_type` values are `company` and `job`. The server
SHALL resolve the slug to the subject's internal id and persist the thread with
`subject_type`, the resolved `subject_id`, a title, and the opening body.

#### Scenario: Create a thread on a company

- **WHEN** a signed-in user POSTs a thread with `subject_type=company`, a valid
  company slug, a title, and a body
- **THEN** the server resolves the slug to the company id, creates the thread and
  its opening reply, and returns the created thread including its persona handle

#### Scenario: Create a thread on a vacancy

- **WHEN** a signed-in user POSTs a thread with `subject_type=job` and a valid
  vacancy slug
- **THEN** the server resolves the slug to the job id and creates the thread

#### Scenario: Reject an unknown subject

- **WHEN** a user POSTs a thread whose slug does not resolve to an existing
  company or job
- **THEN** the server rejects the request with 404 and creates nothing

#### Scenario: Reject an unsupported subject type

- **WHEN** a user POSTs a thread with a `subject_type` other than `company` or
  `job`
- **THEN** the server rejects the request with 400

#### Scenario: Reject anonymous creation

- **WHEN** an unauthenticated request attempts to create a thread
- **THEN** the server rejects it with 401

### Requirement: Anonymous persona identity

Every user who authors a thread or reply SHALL be shown to other users only
through a stable, pseudonymous persona handle. The handle SHALL be the same
across all of that user's threads and replies. The authoring user's real
`user_id` SHALL be stored for moderation and rate limiting but SHALL NEVER appear
in any client-facing response.

#### Scenario: Handle minted on first authored content

- **WHEN** a user authors their first thread or reply and has no persona yet
- **THEN** the server mints a unique handle for that user and reuses it thereafter

#### Scenario: Real identity never exposed

- **WHEN** any thread or reply is serialized to a client
- **THEN** the response contains the persona handle and omits the author's
  `user_id` and any other identifying field

### Requirement: Reply to a thread

A signed-in user SHALL post a reply to an existing thread. Replies are flat and
ordered chronologically. Posting a reply SHALL increment the thread's reply
count.

#### Scenario: Post a reply

- **WHEN** a signed-in user POSTs a reply body to an existing open thread
- **THEN** the server stores the reply against the thread and increments its
  reply count

#### Scenario: Reply to a missing thread

- **WHEN** a user posts a reply to a thread id that does not exist
- **THEN** the server responds 404 and stores nothing

### Requirement: List threads for a subject

The system SHALL return the threads attached to a given subject, newest first,
each carrying its persona handle, title, reply count, and timestamps.

#### Scenario: List a company's threads

- **WHEN** a client requests threads for a `company` subject by slug
- **THEN** the server returns that company's threads newest first, without any
  author `user_id`

### Requirement: Read a thread with its replies

The system SHALL return a single thread with its replies in chronological order,
each reply carrying its persona handle and body.

#### Scenario: Read a thread

- **WHEN** a client requests a thread by id
- **THEN** the server returns the thread and its replies oldest first, each with
  a persona handle and no `user_id`

### Requirement: Per-user rate limiting

The system SHALL limit how many threads and replies a single user creates within
a time window, keyed on the private `user_id`. Requests over the limit SHALL be
rejected without creating content.

#### Scenario: Thread creation over the limit

- **WHEN** a user creates more threads than the allowed number within the window
- **THEN** the server rejects further creations with 429 and stores nothing

### Requirement: Moderator can close a thread

A thread SHALL carry a status that a moderator can set to closed. A closed thread
SHALL be hidden from the default subject listing and SHALL reject new replies.

#### Scenario: Closed thread hidden and locked

- **WHEN** a moderator closes a thread
- **THEN** the thread no longer appears in the subject's default listing and new
  replies to it are rejected
