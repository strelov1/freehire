## ADDED Requirements

### Requirement: Render a subscription digest to an email

The system SHALL render a filter-subscription digest into an email with a subject
naming the saved search and its new-match count, an HTML body, and a plain-text
alternative. Each listed job SHALL link to its on-platform freehire job page
(`<origin>/jobs/<slug>`), never to a source URL. The listed jobs SHALL be capped
to a configured maximum with the remainder summarized as an "and N more" tail, so
a large digest cannot produce an unbounded message. All job-, company-, salary-,
and saved-search-name text SHALL be HTML-escaped in the HTML body because it is
user- or source-derived.

#### Scenario: Digest renders subject, HTML, and text

- **WHEN** a digest of matched jobs is rendered for email
- **THEN** the email has a subject naming the saved search and match count, an HTML body listing each job as a link to its freehire job page, and a plain-text alternative carrying the same information

#### Scenario: Oversized digest is capped with a summary tail

- **WHEN** a digest lists more jobs than the configured cap
- **THEN** the body shows the capped number of jobs followed by an "and N more" summary rather than every job

#### Scenario: User and source text is escaped

- **WHEN** a job title, company, or saved-search name contains HTML-significant characters
- **THEN** the HTML body escapes them so the content cannot inject markup

### Requirement: Send digest email via AWS SES

The system SHALL send the rendered digest email through AWS SES (v2 `SendEmail`)
from a configured sender address to the subscriber's destination address. AWS
credentials SHALL be resolved from the default AWS credential chain, never from
application configuration. A send failure SHALL be reported to the caller (the
delivery engine) as an error so the match retry/dead-letter bookkeeping applies.

#### Scenario: Successful send

- **WHEN** the email notifier sends a digest to a valid recipient with SES reachable
- **THEN** SES `SendEmail` is invoked with the configured From address and the recipient's address, and the notifier reports success

#### Scenario: Send failure surfaces as an error

- **WHEN** the SES `SendEmail` call fails
- **THEN** the notifier returns an error so the delivery is retried and eventually dead-lettered rather than silently dropped

### Requirement: Email channel is enabled only when SES is configured

The email channel SHALL be active only when its SES configuration (region and
sender address) is present. When it is absent, the notification worker SHALL still
deliver other channels, and email subscriptions SHALL be softly skipped (their
matches stay pending, no delivery attempt is counted) rather than failed.

#### Scenario: Unconfigured email channel does not break the worker

- **WHEN** the worker runs with the SES sender/region unset but the Telegram bot configured
- **THEN** Telegram subscriptions are delivered normally and email subscriptions are softly skipped with their matches left pending
