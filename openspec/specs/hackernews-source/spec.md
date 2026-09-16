# hackernews-source Specification

## Purpose
Crawls Hacker News' monthly "Ask HN: Who is hiring?" threads into the catalogue through the
Algolia HN Search API, one posting per top-level comment that follows the thread's own
"Company | Role | ..." header convention.
## Requirements
### Requirement: Hiring threads are found by the posting account, not by free text

The system SHALL provide a boardless `hackernews` source that finds hiring threads by
listing the `whoishiring` account's newest stories and keeping the ones whose title matches
"Who is hiring", newest first, up to two (the current thread, still filling, and the
previous one, still live in the first weeks of the new one). A crawl finding no such thread
SHALL fail rather than silently yield nothing.

#### Scenario: Sibling threads from the same account are skipped

- **WHEN** the account's newest stories include "Who wants to be hired?" and "Freelancer?"
  threads beside the hiring threads
- **THEN** only the hiring threads are read, and only up to two

#### Scenario: No hiring thread is an error, not an empty result

- **WHEN** the account's newest stories contain no thread whose title matches "Who is
  hiring"
- **THEN** the crawl fails rather than reporting zero postings

### Requirement: A whole thread's read failure fails the crawl

The source SHALL be a `fullCatalog` provider: each of the (up to two) hiring threads is read
whole in one request, and a thread that cannot be read SHALL fail the entire crawl rather
than silently yielding a smaller catalogue — since a post from a thread that aged out of the
window is genuinely no longer offered, and only a source-wide close on a failed crawl is
correct here.

#### Scenario: An unreadable thread fails the whole crawl

- **WHEN** one of the (up to two) hiring threads cannot be fetched
- **THEN** the crawl fails and yields no postings from either thread

### Requirement: One posting per comment that follows the header convention

For each top-level comment in a read thread, the source SHALL parse the comment's first
paragraph as a pipe-delimited header — employer, then role, then any later segments — and
yield one job when the header has at least two non-empty segments. A comment with fewer than
two pipe segments, a deleted or empty comment, and a reply (non-top-level comment) SHALL NOT
yield a job.

#### Scenario: A canonical post is mapped

- **WHEN** a comment reads "Modash.io | Senior Product Engineer | Remote (Europe) | ... |
  https://modash.io"
- **THEN** a job is yielded with company "Modash.io", title "Senior Product Engineer",
  location "Remote (Europe)", flagged remote, and its URL is that link

#### Scenario: A post with no pipe header yields nothing

- **WHEN** a comment's first paragraph carries no pipe-delimited segments
- **THEN** no job is yielded for it

#### Scenario: Entity-encoded text is decoded

- **WHEN** a comment's header carries HTML entities (e.g. `&amp;` in a company name)
- **THEN** the yielded job's fields have those entities decoded

### Requirement: A header opening with a role instead of an employer is dropped

A header whose first segment reads as a job title (a seniority word paired with a role noun,
e.g. "Senior Software Engineer, Frontend") rather than an employer name SHALL NOT yield a
job, since every later segment is then shifted by one and filing it would record a role
title as the employer. A seniority word alone (with no accompanying role noun) SHALL NOT
trigger this rejection, since it is also a real employer name in practice (e.g. "Lead Bank",
"Chief Industries").

#### Scenario: A role-first header is dropped

- **WHEN** a comment's header reads "Senior Software Engineer, Frontend | New York, NY
  (In-Office) | Full-time"
- **THEN** no job is yielded for it

#### Scenario: A seniority-sounding employer name is not rejected

- **WHEN** a comment's header reads "Lead Bank | Engineer | Kansas City"
- **THEN** a job is yielded with company "Lead Bank"

### Requirement: Location is the first later segment that isn't a commitment, URL, or salary

The location SHALL be taken from the first header segment after the role that is not a
commitment word (e.g. "full-time", "contract"), a bare URL, or a salary figure.

#### Scenario: Commitment and salary segments are skipped when finding location

- **WHEN** a comment's header reads "Acme | Engineer | Contract | $200k | London"
- **THEN** the yielded job's location is "London"

### Requirement: The link is the post's first anchor, falling back to the comment permalink

The yielded job's URL SHALL be the first absolute `http(s)` link found in the comment,
falling back to the comment's own permalink on Hacker News when the comment has none.

#### Scenario: A post with no link falls back to its permalink

- **WHEN** a comment names a company and role but contains no link
- **THEN** the yielded job's URL is that comment's own Hacker News permalink

