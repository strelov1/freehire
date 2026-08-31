## ADDED Requirements

### Requirement: A transcription consumes a plan allowance

The system SHALL consume one dictation allowance per accepted transcription request,
before the audio is sent upstream. The allowance is enforced in addition to — not instead
of — the existing per-caller rate limit: the rate limit bounds how fast a caller may ask,
and the allowance bounds how much they may have in a day. A transcription that returns no
usable text SHALL return its allowance.

Refusing for want of allowance SHALL be `402` and SHALL be distinguishable from the `429`
the rate limit answers with, because one clears in seconds and the other clears tomorrow.

#### Scenario: Transcription within allowance

- **WHEN** a user with dictation allowance remaining submits audio
- **THEN** the audio is transcribed and one allowance is consumed

#### Scenario: Transcription beyond allowance

- **WHEN** a user who has spent today's dictation allowance submits audio
- **THEN** the system responds `402`, the audio is not sent upstream, and the body names
  when the allowance resets

#### Scenario: Rate limit and allowance are distinct refusals

- **WHEN** a caller with allowance remaining exceeds the per-caller rate limit
- **THEN** the response is `429` and no allowance is consumed

#### Scenario: A failed transcription returns its allowance

- **WHEN** a transcription is charged and the upstream call fails or returns no text
- **THEN** the allowance is returned

#### Scenario: Dictation is absent, not refused, when unconfigured

- **WHEN** transcription is not configured in the environment
- **THEN** the endpoint's existing unconfigured response is returned and no allowance is
  consumed
