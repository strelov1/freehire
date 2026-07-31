## ADDED Requirements

### Requirement: The composer offers dictation where the browser can record

The assistant composer SHALL offer a microphone control that records the caller's voice and
appends its transcription to the draft. The control MUST be absent — not present and failing
— where the browser cannot record: `MediaRecorder` and `navigator.mediaDevices.getUserMedia`
are unavailable outside a secure context and in browsers that do not implement them, and a
control that can only fail teaches people to stop reaching for it.

#### Scenario: A browser without the recording APIs shows no microphone

- **WHEN** the composer renders in a context where `MediaRecorder` or `getUserMedia` is unavailable
- **THEN** no microphone control is rendered

#### Scenario: A supported browser shows the microphone

- **WHEN** the composer renders in a secure context with both APIs present
- **THEN** a microphone control is rendered beside the send control

### Requirement: Dictation writes to the draft and never sends

A completed transcription SHALL be appended to the composer draft, and the caller SHALL be
the one who sends it. Nothing is dispatched on their behalf: a transcription is a guess about
what was said, and a wrong guess sent automatically is a message they cannot unsend.

Appending MUST preserve what the caller had already typed, separated from it by a single
space when the existing draft does not already end in whitespace.

#### Scenario: Dictating into an empty composer

- **WHEN** the draft is empty and a recording transcribes to "find me remote Go jobs"
- **THEN** the draft becomes "find me remote Go jobs" and no turn is started

#### Scenario: Dictating after typed text

- **WHEN** the draft holds "also" and a recording transcribes to "in Berlin"
- **THEN** the draft becomes "also in Berlin"

#### Scenario: An empty transcription changes nothing

- **WHEN** a recording transcribes to an empty string or to whitespace only
- **THEN** the draft is unchanged

### Requirement: A recording can be abandoned without being transcribed

The caller SHALL be able to cancel a recording in progress. A cancelled recording MUST NOT be
uploaded, MUST NOT be transcribed, and MUST leave the draft unchanged.

#### Scenario: Cancelling discards the audio

- **WHEN** the caller cancels a recording in progress
- **THEN** no transcription request is issued and the draft is unchanged

#### Scenario: The microphone is released either way

- **WHEN** a recording ends, whether confirmed or cancelled
- **THEN** every track of the captured stream is stopped

### Requirement: A recording is bounded in length

A recording SHALL stop itself at a fixed ceiling and transcribe what it captured. The ceiling
exists because transcription is billed per minute of audio and a forgotten open microphone is
otherwise unbounded spend, and because an upload past the server's cap would be rejected after
the caller had already spoken.

#### Scenario: Reaching the ceiling ends the recording

- **WHEN** a recording reaches the maximum length
- **THEN** it stops on its own and the captured audio is transcribed

### Requirement: A refused or failed recording is explained

Denied microphone permission, a failed capture, and a failed transcription SHALL each surface
a message the caller can act on, and MUST leave the composer usable by keyboard.

#### Scenario: Permission is denied

- **WHEN** the caller denies microphone permission
- **THEN** the composer reports that the microphone is unavailable and remains typable

#### Scenario: Transcription fails

- **WHEN** the transcription request fails
- **THEN** the failure is reported, the draft is unchanged, and the microphone returns to its idle state

### Requirement: The transcription endpoint

`POST /api/v1/speech/transcriptions` SHALL accept one audio file as `multipart/form-data`
under the part name `file` and respond `{"data": {"text": "..."}}`.

It SHALL authenticate on the same terms as the assistant's message endpoint, so any client
that may post a message to the agent may also transcribe for it. It SHALL be rate limited per
caller rather than per IP, because an IP key is lifted by any rotating proxy pool and the cost
here is a metered upstream.

The request body SHALL be capped below the server's global body limit, and the cap SHALL be
enforced on the bytes read rather than on the size the client declares.

#### Scenario: A recording is transcribed

- **WHEN** an authenticated caller posts an audio file
- **THEN** the response is `200` with the transcribed text under `data.text`

#### Scenario: An unauthenticated caller is refused

- **WHEN** the request carries neither a session cookie nor an accepted key
- **THEN** the response is `401` and no upstream call is made

#### Scenario: A missing part is a bad request

- **WHEN** the request carries no `file` part
- **THEN** the response is `400`

#### Scenario: An oversize upload is refused before the upstream is called

- **WHEN** the uploaded audio exceeds the endpoint's cap
- **THEN** the response is `400` and no upstream call is made

#### Scenario: An unconfigured gateway reports the feature as absent

- **WHEN** no speech gateway is configured
- **THEN** the response is `501` and the composer renders no microphone

#### Scenario: An upstream failure is not reported as the caller's fault

- **WHEN** the speech gateway errors or times out
- **THEN** the response is `502`

#### Scenario: A caller who floods the endpoint is throttled

- **WHEN** one caller exceeds the per-caller rate limit
- **THEN** further requests answer `429` until the window passes
