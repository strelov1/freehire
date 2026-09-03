## ADDED Requirements

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

## MODIFIED Requirements

### Requirement: List threads for a subject

The system SHALL return the threads attached to a given subject, newest first,
each carrying its persona handle, title, reply count, and timestamps.

Paged listings SHALL signal a further page only when one exists: a
continuation cursor SHALL be returned only when the page returned is full, and
SHALL be absent on the final page. This applies to both the subject thread
listing and a thread's reply listing.

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
