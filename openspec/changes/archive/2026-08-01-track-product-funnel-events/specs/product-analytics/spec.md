## MODIFIED Requirements

### Requirement: Explicit funnel events

The app SHALL emit explicit events for the core funnel — `search`, `job_view`,
`job_apply`, `job_save`, `job_track`, `signup`, `cv_upload`, `match_run`,
`tailor_run`, `assistant_message` — through a single analytics module, fired on
the UI action regardless of authentication state. A no-op SHALL be safe when PostHog is uninitialized. Event properties
SHALL carry no personal data: no email, no name, no file name, no résumé or
message text. A failure reason SHALL be reported as a bounded code drawn from a
closed set, never as a raw error message, so that a reworded error does not
silently split a metric into two.

#### Scenario: Anonymous user applies

- **WHEN** an unauthenticated user clicks Apply on a job
- **THEN** a `job_apply` event is captured with the job slug and source

#### Scenario: PostHog uninitialized

- **WHEN** a tracked action fires while PostHog is not initialized
- **THEN** the analytics call is a safe no-op and does not throw

#### Scenario: Account created

- **WHEN** a visitor completes registration, by OAuth provider or by password
- **THEN** a `signup` event is captured carrying the method used and no PII

#### Scenario: CV uploaded successfully

- **WHEN** a user uploads a CV and the server accepts it
- **THEN** a `cv_upload` event is captured marked as successful, with no file
  name and no résumé text

#### Scenario: CV upload rejected

- **WHEN** a CV upload is rejected by the server, for example because the PDF
  carries no extractable text
- **THEN** a `cv_upload` event is captured marked as failed, carrying a bounded
  reason code rather than the server's message text

#### Scenario: Unrecognized failure

- **WHEN** an upload fails with a message the reason mapping does not recognise
- **THEN** the event is still captured with a catch-all reason code, so a failure
  is never silently dropped from the count

#### Scenario: Credit-charged feature invoked

- **WHEN** a user starts a job-match analysis or a CV tailoring session
- **THEN** a `match_run` or `tailor_run` event is captured respectively

#### Scenario: Assistant turn sent

- **WHEN** a user sends a message to the in-app assistant
- **THEN** an `assistant_message` event is captured carrying no message text
