## ADDED Requirements

### Requirement: Multi-device push fan-out

Any notification engine delivering over the `push` channel SHALL send to every
device currently registered for the recipient user, not a single fixed
destination. A delivery counts as sent for that channel as long as at least one
registered device received it; if every device's send fails or the device's
registration turns out to be dead, the delivery SHALL be treated as failed for
that pass (retried on the engine's normal retry schedule) rather than silently
dropped.

#### Scenario: One of several devices is unreachable

- **WHEN** a user has two registered devices and the push relay reports one delivered and one undeliverable
- **THEN** the delivery counts as sent, and the undeliverable device's registration is removed so it is not retried

#### Scenario: Every device is unreachable

- **WHEN** a user has one or more registered devices and none of them accept the push
- **THEN** the delivery counts as failed for that pass and is retried on the engine's normal retry/dead-letter schedule

### Requirement: Push channel needs no server-side credential gate

Unlike `telegram` (needs a bot token) and `email` (needs SES credentials), the
`push` channel SHALL be available in every delivery engine unconditionally — the
push relay holds its own device credentials, so there is no environment
configuration that turns the channel off server-wide. Deliverability is decided
per recipient (has a registered device or not), never per deployment.

#### Scenario: Push channel is always registered

- **WHEN** a notification worker starts up with no push-specific environment variables set
- **THEN** the `push` channel is still available for delivery, unlike `telegram`/`email` which are skipped when unconfigured

### Requirement: Application nudge push delivery

A due application lifecycle nudge (follow-up, interview-prep, or job-closed) whose
account rule includes the `push` channel SHALL be delivered as a short push
notification, distinct per nudge kind, to every device registered for the
account, following the same fan-out and soft-skip rules as the other two
notification engines. Since a nudge always concerns exactly one application/job,
the push SHALL carry that job's slug as deep-link data.

#### Scenario: Follow-up nudge over push

- **WHEN** a follow-up nudge is delivered over `push`
- **THEN** the push's title/body names the job and company and indicates the application has gone quiet, and the data payload includes the job's slug

#### Scenario: Interview-prep nudge over push

- **WHEN** an interview-prep nudge is delivered over `push`
- **THEN** the push's title/body indicates an upcoming interview for that job, and the data payload includes the job's slug

#### Scenario: Job-closed nudge over push

- **WHEN** a job-closed nudge is delivered over `push`
- **THEN** the push's title/body indicates the job was closed, and the data payload includes the job's slug

#### Scenario: No registered device is skipped, not failed

- **WHEN** a nudge's rule includes `push` but the user has no currently registered device
- **THEN** that channel is skipped without failing the nudge, and remaining configured channels still deliver

### Requirement: Tapping a push with a job deep-link opens that job

The mobile app SHALL, on receiving a tap on a push notification whose data payload
includes a job slug, navigate directly to that job's detail screen. A push with no
job slug in its data payload SHALL simply foreground the app without navigating
anywhere in particular.

#### Scenario: Tap with a deep link

- **WHEN** the user taps a push notification carrying a job slug in its data
- **THEN** the app opens directly to that job's detail screen

#### Scenario: Tap without a deep link

- **WHEN** the user taps a push notification carrying no job slug
- **THEN** the app opens to its default screen
