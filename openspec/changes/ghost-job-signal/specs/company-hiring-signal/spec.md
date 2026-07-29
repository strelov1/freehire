## ADDED Requirements

### Requirement: Per-company application response rate, gated by sample size

The company rollup SHALL maintain, alongside the existing hiring-velocity scalars, the number of
applications tracked against a company and how many of them received a reply, and SHALL serve a
response rate derived from the pair **only when the company has at least ten tracked
applications**. Below that sample size the field SHALL be absent from the response, not zero and
not an estimate.

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
