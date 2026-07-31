# Speech to text

## Scope
`internal/speech` — one method that turns recorded audio into text, and nothing else.
Its HTTP surface is `internal/handler/speech.go`; the microphone that feeds it is
`web/src/lib/assistant/VoiceInput.svelte`.

## Always true
- **It is not part of `internal/llm`.** That package is built on langchaingo, which
  models chat completions and has no audio surface; a multipart audio POST bolted onto
  it would reach around the abstraction it exists to provide. What the two DO share is
  the endpoint — an OpenAI-compatible gateway serves `/chat/completions` and
  `/audio/transcriptions` from the same host with the same key — so they are configured
  together and only the model name (`STT_MODEL`) is separate.
- **`STT_MODEL` has no default, and that is the feature switch.** Transcription is
  billed per minute of audio against an assistant that is not metered, so a deployment
  that never named a model gets no microphone rather than a bill for one nobody chose.
- **`New` returns nil when unconfigured.** Nil is "this deployment has no speech",
  following `internal/headshot`: the handler answers 501 and the SPA reads that as a
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
(`internal/credits` is the seam and is not wired to it). Three things stand in for that,
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
