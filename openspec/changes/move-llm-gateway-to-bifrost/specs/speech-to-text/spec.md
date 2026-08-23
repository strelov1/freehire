## MODIFIED Requirements

### Requirement: Audio surfaces are absent while the gateway cannot serve them

The system SHALL NOT offer dictation or voice mode while the configured gateway's audio routes are unproven. The affordance SHALL be absent rather than present-and-failing, and the capability SHALL be restored by configuration and one client-side switch, with no code removed in the meantime.

#### Scenario: No microphone in the composer

- **WHEN** a signed-in user opens the assistant composer in a browser that supports recording
- **THEN** no microphone control is rendered, and no recording can be started

#### Scenario: No voice mode on an interview session

- **WHEN** a signed-in user opens an interview session in a browser that supports WebRTC
- **THEN** no voice-call trigger is offered

#### Scenario: The server still answers an unconfigured surface honestly

- **WHEN** a transcription or realtime-credential request reaches the API while no speech model or realtime gateway is configured
- **THEN** the API answers 501, which the SPA reads as a surface that does not exist here rather than as a fault

#### Scenario: Restoring the capability

- **WHEN** the gateway is given an audio-capable credential, both audio routes are exercised against it, the speech model and realtime configuration are set, and the client-side switch is flipped
- **THEN** dictation and voice mode return, gated as before by the browser capability checks that were left in place
