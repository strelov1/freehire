## MODIFIED Requirements

### Requirement: The ATS cross-check fires only under a coverage gate and expires

The system SHALL stamp `ats_absent` on a job only when all of the following hold: the job's source
is of kind `aggregator`; the job names a company (a non-empty company slug); the job does not
state that it is published on a client's behalf (`via_recruiter`), and its company has fewer than
two open jobs that do; the company has at least five open jobs from sources of kind `ats` or
`company`; and no open job of that company from such a source carries the job's role.

A board job carries the role when the two titles share a key — the normalized title with
punctuation folded to spaces, with or without its trailing clause — or when at least two thirds
of the shorter title's significant words, stemmed, appear in the other. Matching leans toward
"carried" on purpose: a missed absence costs an uncounted criterion, a false one accuses the
employer. The description SHALL NOT take part, since it does not survive an aggregator's
rewriting.

The postings of one company from one aggregator source SHALL be judged together, and none of
them stamped unless the company's board carries at least one of them and no more than half of
them are absent. A board that carries none of what a source lists is not evidence about that
source, and a batch that is mostly absent describes the company — an agency, or a board we
only partly crawl — rather than any one posting.

A stamp is the latest run's verdict: a stamped job that a run does not stamp again SHALL lose
its stamp. A stamp older than 14 days SHALL be ignored. The worker re-stamps on every run, so
an expired stamp means the worker has stopped — and a stopped worker must fall silent rather
than keep accusing the catalogue from a frozen snapshot.

#### Scenario: No company board means no evidence

- **WHEN** a company appears only on aggregator sources, or has fewer than five open jobs on its own boards
- **THEN** no job of that company carries the `ats_absent` criterion, whatever its titles

#### Scenario: The role is present on the company's own board

- **WHEN** an aggregator posting's role matches an open job of the same company from an ATS source
- **THEN** the absence stamp is cleared and the criterion does not fire

#### Scenario: The role is absent from a board we do crawl

- **WHEN** a company's own board carries some of a source's postings for it, and an aggregator posting's role matches none of the board's jobs, and at most half of that source's postings for the company are absent
- **THEN** the posting is stamped absent and the criterion fires

#### Scenario: An aggregator that retitled the role still matches

- **WHEN** an aggregator lists "Account Executive Fraud" while the company's board lists "Account Executive - Fraud"
- **THEN** the role is carried and the criterion does not fire

#### Scenario: A posting with no company is never stamped

- **WHEN** an aggregator posting has an empty company slug
- **THEN** it is skipped, reported as such, and any stamp it carries is cleared

#### Scenario: A recruiter's posting is never stamped

- **WHEN** a posting states it is published on a client's behalf, or its company has at least two open postings that do
- **THEN** it is skipped, reported as such, and any stamp it carries is cleared

#### Scenario: A board that carries none of a source's postings is no evidence

- **WHEN** none of a company's postings from one aggregator source is carried by the company's board
- **THEN** none of them is stamped

#### Scenario: A mostly absent batch is no evidence

- **WHEN** more than half of a company's postings from one aggregator source are absent from its board
- **THEN** none of them is stamped

#### Scenario: An expired stamp yields no evidence

- **WHEN** a job's absence stamp is older than 14 days
- **THEN** the `ats_absent` criterion does not fire
