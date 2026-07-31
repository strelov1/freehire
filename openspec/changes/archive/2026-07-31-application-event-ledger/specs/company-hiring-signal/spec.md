## MODIFIED Requirements

### Requirement: Per-company application response rate, gated by sample size

The company rollup SHALL maintain, alongside the existing hiring-velocity scalars, the number of
applications tracked against a company and how many of them received a reply, and SHALL serve a
response rate derived from the pair **only when the company has at least ten tracked
applications**. Below that sample size the field SHALL be absent from the response, not zero and
not an estimate.

Both sides of the ratio SHALL be counted from `application_events`, not from live `emails` rows.
An application counts as answered when a non-retracted `employer_reply` event exists for it.
Reading the mail table made the served rate a function of the candidate's inbox hygiene: the
rebuild counted messages `WHERE deleted_at IS NULL`, so a candidate clearing old mail made an
employer that had answered them look silent, on a public page, about a named company.

Only applications whose owner has a connected mailbox SHALL count, on both sides of the ratio, for
the same reason the job-level signal requires it: where no mail can be linked, an unanswered
application is a gap in our data rather than an observed silence.

This is the measure a previous investigation identified as the only unconfounded company-level
signal, having ruled out posting age and never-closing as artifacts of company type and of our own
ingest history. It is expected to be absent for nearly every company until the sample matures;
that absence is the correct answer, not an unfinished implementation.

#### Scenario: A company below the sample gate serves no rate

- **WHEN** a company has four tracked applications from users with connected mailboxes
- **THEN** the served company payload carries no response rate

#### Scenario: A company above the gate serves the rate

- **WHEN** a company has twelve tracked applications, three of which received a reply
- **THEN** the served company payload reports a response rate over that denominator

#### Scenario: Applications without a connected mailbox are excluded from both sides

- **WHEN** a company has twenty tracked applications, of which six belong to users with connected mailboxes
- **THEN** the sample size is six, the gate is not met, and no rate is served

#### Scenario: Deleted mail does not move the rate

- **WHEN** a candidate deletes the reply an employer sent them and the rollup runs again
- **THEN** that application still counts as answered and the company's rate is unchanged

#### Scenario: A corrected link moves the answer to the right company

- **WHEN** an email counted against company A is re-linked to company B and the rollup runs
- **THEN** A's answered count drops by one and B's rises by one

## ADDED Requirements

### Requirement: Per-company time to first reply

The company rollup SHALL record the median number of days from application to the first
non-retracted `employer_reply` event, and SHALL serve it under the same ten-application sample
gate as the response rate it accompanies.

Applications that never received a reply SHALL be excluded from the median and reported as a
count beside it. A median over answered applications only is right-censored, and presenting it
without saying how many are still waiting states a reassuring number that the unanswered
majority contradicts.

The median SHALL be computed over `occurred_at`, so importing a year of historical mail does not
compress every past reply into the day the mailbox was connected.

#### Scenario: A company with enough answered applications

- **WHEN** a company has fourteen observable applications, nine of them answered
- **THEN** the payload reports the median days to first reply over those nine, and reports that
  five received no reply

#### Scenario: Above the gate but never answered

- **WHEN** a company has eleven observable applications and none received a reply
- **THEN** the payload reports a response rate of zero and no median, rather than a median of
  zero days

#### Scenario: The median ignores retracted events

- **WHEN** an application's only `employer_reply` event has been retracted by a link correction
- **THEN** that application counts as unanswered in both the rate and the median
