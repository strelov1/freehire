## Why

The assistant chat is open to every signed-in user, but three things about the surface
still work against the conversation it hosts.

Reading during a turn is impossible: `scrollToBottom()` fires on every streamed event
unconditionally, so scrolling up to re-read what the agent said a minute ago is undone
within milliseconds. The longer the answer, the worse it gets — and tailoring answers are
long.

Speaking is impossible: the experience interview asks people to narrate their career, and
narration is what a keyboard is worst at. A candidate who would happily talk for two
minutes about a project types two sentences instead, and the experience bank gets the
two sentences.

Continuing is unguided: a turn ends and the composer is blank. The agent has just returned
a deck of vacancies or a fit verdict, and the obvious next moves — compare the first two,
run my CV against this one — are known to the system but offered nowhere.

## What Changes

- The transcript pane sticks to the bottom only while the reader is already there. Scrolling
  up during a turn holds position; a "jump to latest" control returns. Sending a message
  always returns to the bottom — that is a deliberate act, not an arriving frame.
- A microphone control in the composer records the caller's voice, transcribes it, and
  appends the text to the draft. The caller sends it themselves; nothing is dispatched on
  their behalf.
- A new `POST /api/v1/speech/transcriptions` accepts one audio upload and returns its text.
  It runs against the same OpenAI-compatible gateway the rest of the LLM stack uses, under
  a new `STT_MODEL` (default `whisper-1`). Cookie-authenticated only, per-user rate limited,
  and capped well below the global body limit.
- After a turn settles, up to three follow-up questions render beneath the last answer.
  Clicking one sends it as an ordinary message. They come from a separate cheap model call
  outside the turn loop, so a failure to produce them is invisible rather than fatal.

## Capabilities

### New Capabilities

- `assistant-transcript-scrolling`: when the transcript pane follows the stream and when it
  yields to the reader.
- `assistant-voice-dictation`: recording the caller's voice into the composer draft, and the
  transcription endpoint behind it.
- `assistant-follow-ups`: suggesting the next question after a turn settles.

### Modified Capabilities

None. No existing requirement changes: the scroll behaviour, the composer's microphone and
the follow-up strip are all behaviours no current spec describes.

## Impact

**New code.** `internal/speech` (a thin client for the gateway's `/audio/transcriptions`),
`internal/handler/speech.go`, `internal/handler/assistant_followups.go`,
`web/src/lib/assistant/VoiceInput.svelte`, `web/src/lib/assistant/dictation.ts`,
`web/src/lib/assistant/followups.ts`.

**Modified code.** `web/src/lib/assistant/AssistantChat.svelte` (scroll, follow-up strip),
`web/src/lib/assistant/Composer.svelte` (microphone), `cmd/server/main.go` (wire the speech
client), `internal/config` (`STT_MODEL`), `internal/handler/assistant.go` (route).

**Runtime.** Transcription is billed per minute of audio and follow-ups add one cheap model
call per settled turn. The assistant has no metering (`internal/credits` is the seam and is
not wired to it), so authentication, the per-user rate limit and the upload cap are the only
things bounding this spend. Both features degrade to absent rather than to an error when the
gateway is unconfigured.

**Browsers.** `MediaRecorder` and `getUserMedia` require a secure context. The microphone is
hidden where the API is absent rather than offered and then failing.
