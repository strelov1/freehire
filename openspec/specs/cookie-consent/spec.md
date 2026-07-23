# cookie-consent Specification

## Purpose

Gate every non-essential tracker (Google Analytics, PostHog) behind explicit
consent for visitors in a consent-required region (EU/EEA/UK), detected from the
browser timezone without any IP geolocation. Owns region detection, consent
persistence and state, the banner UI, and its withdrawal entry point.

## Requirements

### Requirement: Consent-required region detection

The frontend SHALL determine, on the client, whether a visitor is in a
consent-required region (EU/EEA/UK) from the browser timezone
(`Intl.DateTimeFormat().resolvedOptions().timeZone`), treating `Europe/*` and the
EEA Atlantic zones as consent-required. The determination MUST NOT use the
visitor's IP address and MUST NOT depend on any server-side geolocation. When the
timezone is unavailable or unrecognized, the visitor SHALL be treated as
consent-required (fail toward asking).

#### Scenario: Visitor in a European timezone

- **WHEN** the browser timezone is `Europe/Berlin`
- **THEN** the visitor is classified as consent-required

#### Scenario: Visitor outside the region

- **WHEN** the browser timezone is `America/New_York`
- **THEN** the visitor is classified as not consent-required

#### Scenario: Timezone unavailable

- **WHEN** the browser timezone cannot be resolved
- **THEN** the visitor is treated as consent-required

### Requirement: Consent persistence

The frontend SHALL persist a visitor's cookie choice in `localStorage` under a
single key holding one of `granted` or `denied`, and SHALL treat the absence of
that key as "no choice made". The stored choice SHALL survive reloads and be
readable synchronously at boot.

#### Scenario: Choice recorded

- **WHEN** the visitor accepts or rejects
- **THEN** the corresponding value (`granted` / `denied`) is written to the
  `localStorage` key

#### Scenario: Returning visitor

- **WHEN** the app boots and the key already holds `granted` or `denied`
- **THEN** the stored choice is used and the banner is not shown again

### Requirement: Non-essential trackers gated on consent

For a consent-required visitor, the frontend SHALL NOT initialize any
non-essential tracker (Google Analytics or PostHog) until consent is `granted`.
A visitor who is not consent-required SHALL have trackers initialized on load
without a banner. When consent is `denied`, no non-essential tracker SHALL load
and no tracking cookie SHALL be set.

#### Scenario: Consent-required, no choice yet

- **WHEN** a consent-required visitor boots the app with no stored choice
- **THEN** neither Google Analytics nor PostHog is initialized

#### Scenario: Consent-required, granted

- **WHEN** a consent-required visitor has (or gives) `granted` consent
- **THEN** both Google Analytics and PostHog are initialized

#### Scenario: Consent-required, denied

- **WHEN** a consent-required visitor has `denied` consent
- **THEN** no non-essential tracker initializes and no tracking cookie is set

#### Scenario: Not consent-required

- **WHEN** a visitor outside the region boots the app
- **THEN** trackers initialize on load and no banner is shown

### Requirement: Consent banner

The frontend SHALL render a consent banner only when the visitor is
consent-required and has made no choice. The banner SHALL present an Accept and a
Reject control of equal visual prominence, with no option pre-selected, and a
link to the privacy policy. Accepting SHALL record `granted` and start the
trackers immediately; rejecting SHALL record `denied` and start nothing.

#### Scenario: Banner shown

- **WHEN** a consent-required visitor has made no choice
- **THEN** the banner is rendered with equal-prominence Accept and Reject
  controls, neither pre-selected, and a privacy-policy link

#### Scenario: Accept

- **WHEN** the visitor clicks Accept
- **THEN** consent is recorded as `granted`, the trackers initialize, and the
  banner is dismissed

#### Scenario: Reject

- **WHEN** the visitor clicks Reject
- **THEN** consent is recorded as `denied`, no tracker initializes, and the banner
  is dismissed

### Requirement: Consent withdrawal entry point

The frontend SHALL provide a persistent entry point (a footer "Cookie settings"
link) that re-opens the banner so a visitor can change a previously recorded
choice, making withdrawal as easy as granting.

#### Scenario: Re-open from footer

- **WHEN** a visitor who previously chose activates the footer "Cookie settings"
  link
- **THEN** the banner re-opens allowing them to change their choice
