## Purpose

Crawls the monthly "Ask HN: Who is hiring?" thread and turns each comment into either a
permanent board contribution (when it names a company via a recognizable ATS link) or a
directly-extracted job posting (when it is free-form vacancy text with no such link), so
this source's postings are never treated as a one-off aggregator feed for a company we
could otherwise crawl properly.

## ADDED Requirements

### Requirement: The current "Who is hiring?" thread is discovered fresh each run

The crawl SHALL discover the current "Ask HN: Who is hiring?" thread via the public Algolia
HN Search API at run time rather than reading it from a configured list, since a new thread
is posted monthly and there is exactly one to crawl at any time.

#### Scenario: The latest thread is found without configuration

- **WHEN** the crawl runs
- **THEN** it queries Algolia for the most recent "Who is hiring?" story and proceeds to
  its comments, with nothing to configure beforehand

### Requirement: Top-level comments are stored idempotently, keyed by the comment's own id

The crawl SHALL store each top-level comment of the discovered thread as a durable record,
keyed by the comment's own globally-unique id, inserting new comments and leaving an
already-stored comment untouched, so re-crawling the same thread never re-processes a seen
comment.

#### Scenario: Re-crawl does not duplicate or reset a stored comment

- **WHEN** a comment already stored (extracted or still pending) appears again in a crawl
- **THEN** the stored record is unchanged

### Requirement: A comment naming a recognizable ATS board becomes a board contribution

The extraction stage SHALL check every URL in a comment against the same ATS-board
recognition the site's own contribution flow uses. A comment where at least one URL
resolves to a recognized provider and board SHALL be submitted as a pending board
contribution instead of a one-off job posting, so the named company joins the regular
first-party crawl and every future posting of theirs is reachable, not only the one this
comment mentioned.

#### Scenario: A comment with a recognized ATS link contributes a board

- **WHEN** a stored comment's text contains a URL that resolves to a known ATS provider and
  board
- **THEN** that (provider, board) is submitted as a pending board contribution, and the
  comment is marked extracted without also producing a directly-written job posting

#### Scenario: An already-tracked board is not duplicated

- **WHEN** a comment names a board already present in the catalog
- **THEN** no duplicate board is created, and the comment is still marked extracted

### Requirement: A comment with no recognizable ATS link is extracted into a job posting

A comment where no URL resolves to a recognized ATS board SHALL be treated as free-form
vacancy text and passed through LLM extraction into the job catalogue directly, with the
posting's identity keyed by the comment's own id so a re-run never creates a duplicate.

#### Scenario: Free-form vacancy text becomes a job posting

- **WHEN** a stored comment names a company and role in prose with no recognizable ATS link
- **THEN** an LLM extraction is attempted and, on success, a job posting is written keyed to
  that comment's id

#### Scenario: An extraction yielding no usable job writes nothing

- **WHEN** an LLM extraction of a comment produces no job with both a title and a
  description
- **THEN** no job posting is written for that comment, and it is still marked extracted

### Requirement: A comment is graded once — never both a contribution and a direct posting

The extraction stage SHALL make exactly one of the two decisions above per comment, never
both: a comment counted as a board contribution SHALL NOT also produce a directly-extracted
job posting, even if its text also happens to describe the role in prose.

#### Scenario: A comment with both a link and descriptive prose only contributes the board

- **WHEN** a comment contains a recognized ATS link AND free-form role description in the
  same text
- **THEN** only the board contribution happens; no job posting is written from that comment

### Requirement: Postings from this source are closed by age, not by a liveness signal

A job posting from this source SHALL be excluded from the liveness probe and instead
closed once it is older than the same age threshold other signal-less sources use, since the
stored reference for such a posting outlives the vacancy itself and a probe can never reach
a death verdict for it.

#### Scenario: An old, still-unclosed posting from this source is closed by age

- **WHEN** a posting from this source has been open longer than the age threshold and has
  not otherwise been closed
- **THEN** it is closed, the same way another signal-less source's stale postings are

### Requirement: A single comment's processing failure does not abort the run

A comment that fails to process (a fetch error, a malformed LLM response, a transient
board-contribution failure) SHALL be retried on a later run rather than aborting the whole
extraction pass, and SHALL NOT be silently dropped.

#### Scenario: One failing comment does not stop the rest of the run

- **WHEN** one stored comment's extraction fails while others in the same run succeed
- **THEN** the failing comment is left unextracted for a later retry, and every other
  comment in the run is still processed
