## MODIFIED Requirements

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
