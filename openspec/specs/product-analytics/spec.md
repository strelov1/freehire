# product-analytics Specification

## Purpose
TBD - created by archiving change add-posthog-analytics. Update Purpose after archive.
## Requirements
### Requirement: Env-gated PostHog initialization

The frontend SHALL initialize the PostHog client only when `PUBLIC_POSTHOG_KEY`
is set, and SHALL remain fully inert (no network calls, no globals mutated) when
it is absent. Initialization SHALL configure a same-origin API host and
`identified_only` person profiles. For a consent-required visitor (see the
`cookie-consent` capability), initialization SHALL additionally require that
consent is `granted`; a consent-required visitor without granted consent SHALL
leave PostHog inert even when `PUBLIC_POSTHOG_KEY` is present.

#### Scenario: Key present

- **WHEN** the client app boots with `PUBLIC_POSTHOG_KEY` set and the visitor is
  not consent-required (or has granted consent)
- **THEN** PostHog is initialized with `api_host` pointing at the same-origin
  reverse-proxy path and `person_profiles: 'identified_only'`

#### Scenario: Key absent

- **WHEN** the client app boots without `PUBLIC_POSTHOG_KEY` (e.g. local dev)
- **THEN** PostHog is not initialized and no analytics network requests are made

#### Scenario: Consent-required without consent

- **WHEN** the client app boots with `PUBLIC_POSTHOG_KEY` set but the visitor is
  consent-required and has not granted consent
- **THEN** PostHog is not initialized and no analytics network requests are made

### Requirement: Same-origin reverse proxy

PostHog event ingestion and session-replay assets SHALL be served through a
same-origin path (`/ingest/`) that proxies to the EU PostHog instance, so that
no external host is added to the Content-Security-Policy and ad-blockers cannot
drop the traffic.

#### Scenario: Events go through the proxy

- **WHEN** the client captures any event
- **THEN** the request targets the same-origin `/ingest/` path, not an external
  `*.posthog.com` host

### Requirement: SPA pageview capture

Because automatic pageview capture is disabled, the app SHALL capture a
`$pageview` event on every client-side navigation.

#### Scenario: Client-side navigation

- **WHEN** the user navigates between routes without a full page reload
- **THEN** a `$pageview` event is captured for the new route

### Requirement: Identity binding without PII

The app SHALL identify a signed-in user to PostHog by user id only, never
sending email or other PII, and SHALL reset identity when the user signs out.

#### Scenario: Signed-in user

- **WHEN** `page.data.user` is present after navigation
- **THEN** the user is identified by id only, with no email in the identify call

#### Scenario: Signed-out transition

- **WHEN** the user transitions from signed-in to signed-out
- **THEN** `posthog.reset()` is called so subsequent events are anonymous

### Requirement: Privacy-scoped session replay

Session replay SHALL be enabled with all inputs masked, and SHALL be disabled on
private routes (`/my/*` and the inbox) so that sensitive content (résumé,
tracking, email) is never recorded.

#### Scenario: Public route

- **WHEN** the user is on a public route (e.g. `/jobs`)
- **THEN** session recording is active with input values masked

#### Scenario: Private route

- **WHEN** the user navigates to a route under `/my` or the inbox
- **THEN** session recording is stopped for the duration of that route

### Requirement: Explicit funnel events

The app SHALL emit explicit events for the core funnel — `search`, `job_view`,
`job_apply`, `job_save`, `job_track`, `signup`, `cv_upload`, `match_run`,
`tailor_run`, `assistant_message` — through a single analytics module, fired on
the UI action regardless of authentication state. A no-op SHALL be safe when
PostHog is uninitialized. Event properties SHALL carry no personal data: no
email, no name, no file name, no résumé or message text. A failure reason SHALL
be reported as a bounded code drawn from a closed set, never as a raw error
message, so that a reworded error does not silently split a metric into two.

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

### Requirement: Client feature flag reader

The app SHALL expose a generic client-side feature-flag reader that returns a
PostHog flag's value when available and a caller-supplied fallback otherwise.
Wiring specific product defaults (e.g. `default-hide-nontech`) to flags is out of
scope here and left as a seam until those features land.

#### Scenario: Flag resolves

- **WHEN** a caller reads a feature flag that PostHog has loaded
- **THEN** the reader returns the flag's value

#### Scenario: Flag unavailable

- **WHEN** the flag cannot be resolved (PostHog inert or flags not loaded)
- **THEN** the reader returns the caller-supplied fallback with no error

