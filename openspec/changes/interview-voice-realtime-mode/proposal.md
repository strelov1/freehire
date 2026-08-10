## Why

The `interview` rehearsal preset is text-only today: the candidate types, the agent
replies as text, and any voice input is just dictation into that same text box. A
throwaway spike (browser mic + VAD + Whisper + ElevenLabs cascade, then OpenAI's
Realtime API over WebRTC) validated that a genuinely hands-free, spoken conversation
feels close to a real interview call and is only practical with the Realtime API — the
cascaded STT→LLM→TTS approach carries multi-second dead air per turn because Whisper
transcription is batch, not streaming, and does not improve with tuning. Rehearsing an
interview by typing answers is a weaker practice signal than saying them out loud under
time pressure, which is the actual skill being trained.

## What Changes

- Add a "Voice mode" entry point to the `interview` preset's session view: a hands-free,
  spoken back-and-forth with the interviewer persona, replacing typed turns for the
  duration of the call.
- Add a backend endpoint that mints a short-lived OpenAI Realtime API client secret
  scoped to one interview session, with the same `interview_context` (vacancy, stage,
  requirements with evidence, employer invitation) the text mode already injects,
  carried as the session's `instructions` instead of a system message.
- The browser connects directly to OpenAI over WebRTC with that secret; audio does not
  transit our backend.
- Each completed turn (candidate line, agent line) is appended to the session's existing
  message history as it completes, so the transcript reads the same whether the
  candidate typed or spoke, and a candidate can switch to typing mid-call without
  losing what was said out loud.
- **Scope cut for v1**: voice mode is rehearsal-only. No tool calls are available in a
  voice turn, so nothing (e.g. a new experience-bank atom) can be written to the
  candidate's profile from voice — only from the text chat, unchanged from today.

## Capabilities

### New Capabilities
- `interview-voice-mode`: hands-free spoken rehearsal over OpenAI's Realtime API for an
  existing `interview` session — the token-minting endpoint, the WebRTC entry point, the
  turn-by-turn transcript persistence into session history, and the v1 no-tool-calls
  constraint.

### Modified Capabilities
(none — existing `interview-rehearsal` requirements about session creation and the
opening brief are unchanged; voice mode is an additional way to converse inside a
session that already exists)

## Impact

- **Backend**: new handler + route under `internal/handler` (mint realtime token,
  scoped like the other `interview` preset endpoints); a turn-append endpoint or reuse
  of the existing message-write path; a new env-gated credential for the Realtime API
  (raw OpenAI, or proxied through litellm if that turns out to support WebRTC — open
  question for the design/implementation phase).
- **Frontend**: new voice-mode UI in the interview session view (`web/src/lib/...`),
  WebRTC connection handling, no manual VAD/STT/TTS plumbing (the Realtime API owns
  turn-taking, barge-in, and speech synthesis).
- **Cost/spend**: Realtime audio is billed per minute, not per token — a new spend
  shape alongside the existing per-user `internal/llmkey` attribution, which assumes
  litellm-routed calls.
- **No DB schema change expected**: turns persist through the existing session message
  storage.
