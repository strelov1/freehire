# community-threads Specification

## Purpose
An anonymous discussion primitive: a signed-in user opens a topic attached to a
subject — a company or a vacancy — and any signed-in user replies, everyone
shown only through a stable pseudonymous handle. The subject is polymorphic and
deliberately unconstrained by a foreign key, so a thread outlives what it is
about and a future subject kind needs no schema change. Threads are readable by
anyone; writing needs a session, and is rate-limited per user.
## Requirements
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

Paged listings SHALL return a continuation cursor only when the page came back
full, and SHALL omit it on a partial page. This applies to both the subject
thread listing and a thread's reply listing.

A full page is not proof that another exists: a listing holding exactly a
multiple of the page size still ends on a full one. That residue is deliberate
and bounded — see
[the design note](../../changes/archive/2026-09-03-add-discussions-feed/design.md)
— and closing it needs a fetch-ahead the domain's listing methods do not do. The requirement is therefore stated as
"full page", not as "a further page exists"; the two differ only in that case,
and stating the stronger one would describe behaviour nothing implements.

#### Scenario: List a company's threads

- **WHEN** a client requests threads for a `company` subject by slug
- **THEN** the server returns that company's threads newest first, without any
  author `user_id`

#### Scenario: A partial page reports no continuation

- **WHEN** a listing returns fewer rows than one page holds
- **THEN** the response omits the continuation cursor

#### Scenario: A full page reports a continuation

- **WHEN** a listing returns exactly as many rows as one page holds
- **THEN** the response carries a continuation cursor positioned at the last returned row

#### Scenario: A thread's replies report the end of the list

- **WHEN** a client reads a thread whose replies fit in fewer rows than one page holds
- **THEN** the response omits the continuation cursor for replies

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

### Requirement: Authored content survives its author's deletion

A thread or reply SHALL remain readable after its author's account is deleted.
Deleting an account SHALL NOT remove discussion other members contributed to.

- A thread whose author is gone SHALL keep its replies, including replies by
  members who still have accounts.
- A de-authored thread or reply SHALL still appear in listings and thread reads;
  the missing author SHALL NOT cause it to be filtered out.
- The author identity of de-authored content SHALL be rendered as an explicit
  deleted-member marker, distinct from both a live persona handle and the AI
  persona used for authorless system replies.

#### Scenario: Thread outlives its author

- **WHEN** a member who opened a thread with replies from others deletes their account
- **THEN** the thread and all its replies remain readable, and the thread still appears in its subject's listing

#### Scenario: De-authored content is not mistaken for the AI persona

- **WHEN** a client reads a thread or reply whose author's account was deleted
- **THEN** the author is presented as a deleted member, distinguishable from the AI persona

#### Scenario: The departed member's handle is gone

- **WHEN** a member's account is deleted
- **THEN** their persona handle no longer appears on any of their surviving threads or replies, and the handle carries no link back to them

### Requirement: List open threads across all subjects

The system SHALL return every open discussion thread, regardless of subject,
newest first, keyset-paged, via `GET /api/v1/threads/recent`. The endpoint SHALL
be public — like the other thread read paths it exposes only persona handles —
and each row SHALL carry its subject's display name in addition to the thread's
own fields, so a client can name the subject without a further request.

A vacancy subject resolves to the posting's title and its company; a company
subject resolves to the company's name. The subject reference is a slug with no
foreign key, so a subject that no longer exists SHALL leave the display name
empty rather than removing the thread from the listing.

#### Scenario: Threads from different subjects appear in one listing

- **WHEN** a client requests the cross-subject listing and open threads exist on both a company and a vacancy
- **THEN** the response contains threads of both subject types, ordered newest first, each carrying its `subject_type`, `subject_slug`, and resolved display name

#### Scenario: A vacancy thread names its posting and company

- **WHEN** the listing includes a thread whose subject is an existing vacancy
- **THEN** that row carries the vacancy's title and the vacancy's company name

#### Scenario: A company thread names the company

- **WHEN** the listing includes a thread whose subject is an existing company
- **THEN** that row carries the company's name

#### Scenario: A thread whose subject was deleted still appears

- **WHEN** the listing includes a thread whose subject slug no longer resolves to any vacancy or company
- **THEN** the thread is still returned, with an empty display name, and is not filtered out

#### Scenario: Closed threads are excluded

- **WHEN** a moderator has closed a thread
- **THEN** that thread does not appear in the cross-subject listing

#### Scenario: Reading the listing requires no session

- **WHEN** an unauthenticated client requests the cross-subject listing
- **THEN** the request succeeds and no response field identifies any author beyond their persona handle

### Requirement: A discussion page names the subject it is about

Every discussion surface scoped to one subject — the thread list and a single
thread, on both subject kinds — SHALL open with that subject: its logo, its
name, and for a vacancy its employer and whether the posting has closed. The
header SHALL be the way back to the subject, replacing any separate breadcrumb.

The subject SHALL be resolved independently of the threads, since a subject
with no threads still has a header to draw. When it cannot be resolved the page
SHALL still render the discussion, and SHALL distinguish a subject that is gone
from one it merely failed to read.

#### Scenario: A vacancy's discussion names the posting

- **WHEN** a reader opens a vacancy's thread list or one of its threads
- **THEN** the page opens with the vacancy's title, its employer and its logo, as a single link to the posting

#### Scenario: A closed vacancy says so

- **WHEN** the vacancy the discussion hangs off has closed
- **THEN** the header marks it closed, without the reader having to follow the link

#### Scenario: A company's discussion names the company

- **WHEN** a reader opens a company's thread list or one of its threads
- **THEN** the page opens with the company's name and logo, as a single link to the company, and does not print its slug

#### Scenario: An absent subject is stated, not linked

- **WHEN** the subject cannot be resolved
- **THEN** the header renders the stored slug as an identifier and is NOT a link, since the destination is the page that could not be read

#### Scenario: Unreachable is not reported as gone

- **WHEN** the subject could not be fetched for a reason other than it being absent
- **THEN** the header says it could not be loaded, distinctly from saying it is no longer listed

#### Scenario: A merged company slug still names its company

- **WHEN** the subject is a company whose slug a merge has retired
- **THEN** the header names the company that absorbed it, and the discussion stays at the url it is under — threads record the retired slug, so the canonical url would not resolve them

### Requirement: A discussions section on the web client

The web client SHALL serve a `/discussions` page rendering the cross-subject
listing, server-rendered on first load and paged from the client thereafter.
Each row SHALL link to that thread's existing page under its own subject. The
page SHALL be reachable from site navigation and SHALL NOT offer starting a
topic, which requires a subject.

#### Scenario: The section lists threads and links to them

- **WHEN** a reader opens `/discussions`
- **THEN** the page lists open threads newest first, names each thread's subject, and each row links to that thread's page under its subject's route

#### Scenario: A thread with an unresolvable subject is readable

- **WHEN** the listing contains a thread whose subject no longer exists
- **THEN** the row renders the stored subject slug in place of a display name and still links to the thread

#### Scenario: Every row states which kind of subject it is about

- **WHEN** a reader scans the listing
- **THEN** each row is marked as being about a vacancy or about a company — on both kinds, so the marker reads as the row's type rather than as a remark about one of them

#### Scenario: A stored slug is not presented as a name

- **WHEN** a row falls back to the stored subject slug
- **THEN** it is presented as an identifier rather than in the styling a resolved employer name is given, and no company logo is resolved from it

#### Scenario: A vacancy with no recorded employer is not called unresolved

- **WHEN** the listing includes a thread on an existing vacancy whose employer name is absent
- **THEN** the row names the posting and marks the employer as unknown, and does NOT fall back to the slug — the subject resolved

#### Scenario: An unreachable feed is not reported as an empty one

- **WHEN** the page cannot fetch the listing
- **THEN** it says the discussions could not be loaded, distinctly from the message shown when the catalogue genuinely holds none

#### Scenario: A failed continuation can be retried

- **WHEN** fetching a further page fails
- **THEN** the failure is shown and the continuation remains available, rather than the list appearing to have ended
