## ADDED Requirements

### Requirement: Env-gated PostHog initialization

The frontend SHALL initialize the PostHog client only when `PUBLIC_POSTHOG_KEY`
is set, and SHALL remain fully inert (no network calls, no globals mutated) when
it is absent. Initialization SHALL configure a same-origin API host and
`identified_only` person profiles.

#### Scenario: Key present

- **WHEN** the client app boots with `PUBLIC_POSTHOG_KEY` set
- **THEN** PostHog is initialized with `api_host` pointing at the same-origin
  reverse-proxy path and `person_profiles: 'identified_only'`

#### Scenario: Key absent

- **WHEN** the client app boots without `PUBLIC_POSTHOG_KEY` (e.g. local dev)
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
`job_apply`, `job_save`, `job_track` — through a single analytics module, fired
on the UI action regardless of authentication state. A no-op SHALL be safe when
PostHog is uninitialized.

#### Scenario: Anonymous user applies

- **WHEN** an unauthenticated user clicks Apply on a job
- **THEN** a `job_apply` event is captured with the job slug and source

#### Scenario: PostHog uninitialized

- **WHEN** a tracked action fires while PostHog is not initialized
- **THEN** the analytics call is a safe no-op and does not throw

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
