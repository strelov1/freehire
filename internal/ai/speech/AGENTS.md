# Speech to text

## Scope
`internal/ai/speech` — one method that turns recorded audio into text, and nothing else.
Its HTTP surface is `internal/api/handler/speech.go`; the microphone that feeds it is
`web/src/lib/assistant/VoiceInput.svelte`.

## Dark since 2026-08-22 — the gateway moved out from under it

Audio is **not served** right now, and the reason is not in this package. The
OpenAI-compatible gateway both this and voice mode depend on is being replaced
(`provision/bifrost/` in freehire-ops). The replacement exposes
`/v1/audio/transcriptions` and `/v1/realtime/client_secrets`, but neither has ever
been called against it: the key pool behind it carries no OpenAI credential, which
makes those routes unproven rather than known-good.

Nothing here was deleted or edited to achieve that — the off switch was already the
design. `STT_MODEL` unset means `New` returns nil, the handler answers 501, and the
same holds for `internal/api/realtime`. What was added is a matching switch in the SPA
(`web/src/lib/assistant/audioAvailability.ts`, `AUDIO_ENABLED`), because without it the
microphone renders, records, fails once and only then latches away — a worse first
impression than no microphone.

**To bring it back:** give the gateway an audio-capable credential, exercise both routes
against it, then set `STT_MODEL` (and the Realtime config) and flip `AUDIO_ENABLED`. One
wire-shape difference is already known and will bite: that gateway wants
`session.model` as `provider/model` on the client-secret route, which is not the shape
sent today.

## Always true
- **It is not part of `internal/platform/llm`.** That package is built on langchaingo, which
  models chat completions and has no audio surface; a multipart audio POST bolted onto
  it would reach around the abstraction it exists to provide. What the two DO share is
  the endpoint — an OpenAI-compatible gateway serves `/chat/completions` and
  `/audio/transcriptions` from the same host with the same key — so they are configured
  together and only the model name (`STT_MODEL`) is separate.
- **`STT_MODEL` has no default, and that is the feature switch.** Transcription is
  billed per minute of audio against an assistant that is not metered, so a deployment
  that never named a model gets no microphone rather than a bill for one nobody chose.
- **`New` returns nil when unconfigured.** Nil is "this deployment has no speech",
  following `internal/candidate/headshot`: the handler answers 501 and the SPA reads that as a
  surface that does not exist here rather than as a fault. Whoever wires it must put an
  untyped nil into the handler's `transcriber` interface — a nil `*Client` inside an
  interface is not a nil interface, and that mistake turns an absent feature into a
  panic on the first recording.
- **An empty transcription is a success.** It is what silence transcribes to. Nothing
  in this stack may treat it as failure; the composer appends nothing and says nothing.
- **Every gateway-side failure wraps `ErrUpstream`** and renders as 502. The caller of
  the API did nothing wrong and has no remedy.

## The filename is not the client's
The gateway identifies the container by file extension, so a name has to travel — but
not the one the browser sent. Go's `multipart.Writer` escapes quotes in a filename and
**not CRLF**, so a crafted name would inject header lines into the request this server
makes. `handler.safeAudioFilename` rebuilds it from an allowlisted extension
(`webm`/`mp4`/`m4a`/`ogg`/`wav`/`mp3`) and refuses anything else with a 400. Adding a
container to the browser's preference list in `dictation.ts` without adding it here is
a 400 the caller cannot explain.

## What bounds the spend
Transcription is billed per minute of audio, and a turn is not metered
(`internal/ai/plan` meters it: one dictation allowance per accepted recording, returned when nothing usable comes back). Three things stand in for that,
none of which depends on metering landing first:

- a per-**caller** rate limit (`transcriptionsPerHour`), keyed on the user rather than
  the address, because an IP key is lifted by any rotating proxy pool;
- a 2 MiB body cap enforced on the bytes read, not on the size the client declares;
- a client-side recording ceiling (`MAX_RECORDING_MS`) well under that cap, so the cap
  is a backstop rather than something a caller discovers after they finish speaking.

## Limitations
- One request per recording: no streaming, no partial results. A caller sees nothing
  until they stop talking.
- No language hint is sent. Whisper detects it, which is why the browser's
  `SpeechRecognition` was not used — that one needs `lang` chosen in advance.
