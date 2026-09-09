# email-notify Specification

## Purpose
TBD - created by archiving change email-notification-channel. Update Purpose after archive.
## Requirements

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
from a configured sender address to the subscriber's destination address, carrying
the `List-Unsubscribe` and `List-Unsubscribe-Post` headers required of a bulk sender
for every mail that is not marked essential. AWS credentials SHALL be resolved from
the default AWS credential chain, never from application configuration. A send
failure SHALL be reported to the caller (the delivery engine) as an error so the
match retry/dead-letter bookkeeping applies.

#### Scenario: Successful send

- **WHEN** the email notifier sends a digest to a valid recipient with SES reachable
- **THEN** SES `SendEmail` is invoked with the configured From address and the recipient's address, and the notifier reports success

#### Scenario: Send failure surfaces as an error

- **WHEN** the SES `SendEmail` call fails
- **THEN** the notifier returns an error so the delivery is retried and eventually dead-lettered rather than silently dropped

#### Scenario: A non-essential mail carries the unsubscribe headers

- **WHEN** the notifier sends any mail not marked essential
- **THEN** the SES call carries a `List-Unsubscribe` header holding the recipient's own unsubscribe URL, and a `List-Unsubscribe-Post: List-Unsubscribe=One-Click` header
- **AND** no `mailto:` alternative, which is optional in RFC 8058 and is what Gmail and Yahoo do not ask of a bulk sender — an advertised address that bounces is worse than an absent one

#### Scenario: Essential mail carries no unsubscribe headers

- **WHEN** the notifier sends an address-verification or password-reset mail
- **THEN** the SES call carries neither header

### Requirement: Email channel is enabled only when SES is configured

The email channel SHALL be active only when its SES configuration (region and
sender address) is present. When it is absent, the notification worker SHALL still
deliver other channels, and email subscriptions SHALL be softly skipped (their
matches stay pending, no delivery attempt is counted) rather than failed.

#### Scenario: Unconfigured email channel does not break the worker

- **WHEN** the worker runs with the SES sender/region unset but the Telegram bot configured
- **THEN** Telegram subscriptions are delivered normally and email subscriptions are softly skipped with their matches left pending

### Requirement: One send path carries every optional part of a message

The mail transport SHALL expose a single send operation taking a message value that
carries the sender, recipient, subject, HTML and text bodies, and the optional
reply-to address, headers, and attachments together. Adding a further optional part
SHALL NOT require a new send method.

#### Scenario: A message with no optional parts sends cleanly

- **WHEN** a message is sent carrying only sender, recipient, subject, and bodies
- **THEN** no reply-to, custom header, or attachment is included in the SES call

#### Scenario: Reply-to, headers, and attachments travel together

- **WHEN** a message is sent carrying a reply-to address, unsubscribe headers, and an attachment
- **THEN** all three reach the SES call in one send, without a separate method per combination

### Requirement: The mail footer renders the unsubscribe URL it is given

The shared mail shell SHALL render, in the footer of a non-essential mail, the
unsubscribe URL supplied by the caller alongside the link to notification settings.
For an essential mail it SHALL render neither. The shell SHALL NOT construct or sign
that URL itself; it renders what it is handed.

#### Scenario: A non-essential mail footer offers both links

- **WHEN** a mail is rendered with an unsubscribe URL and is not marked essential
- **THEN** the footer shows an "Unsubscribe" link to that URL and a link to notification settings

#### Scenario: An essential mail footer offers neither

- **WHEN** a mail is rendered as essential
- **THEN** the footer shows no unsubscribe link and no settings link
