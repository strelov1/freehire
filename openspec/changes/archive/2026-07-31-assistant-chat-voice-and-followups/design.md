## Context

The assistant chat is one Svelte component (`AssistantChat.svelte`) hosting four presets over
an SSE turn stream, backed by `internal/assistant`. Three pieces are being added around that
loop, none of them inside it. All three were mined from `open-webui` (SvelteKit, same stack),
read for approach only — its licence attaches a branding condition that is incompatible with
this repository's MIT terms, so nothing is copied.

Two constraints shape every decision here:

- **The assistant is unmetered.** `internal/credits` exists but is not wired to a turn. Two of
  the three features add per-use upstream cost, so each needs its own bound rather than a
  metering layer that does not exist yet.
- **Model output is untrusted.** The agent reads job descriptions and browsed pages. A
  follow-up question is model output that becomes the *caller's* words on click, which is a
  stronger claim than an answer bubble makes.

## Goals / Non-Goals

**Goals:**

- Reading the transcript during a streaming turn is possible.
- Speaking into the composer is possible wherever the browser can record.
- A settled turn suggests what to ask next, cheaply, and fails invisibly.
- Each new upstream cost has a bound that does not depend on metering landing first.

**Non-Goals:**

- Text-to-speech. Reading answers aloud is a separate feature with its own queueing problem.
- A hands-free "call" mode: voice-activity detection, barge-in, auto-send. That is the
  expensive, finicky half of voice and it only makes sense once dictation and TTS both exist.
- Per-turn metering. The seam stays where it is.
- Editing or regenerating past messages, and conversation branching.
- Persisting follow-ups in the transcript.

## Decisions

### Whisper over the existing gateway, not the browser's `SpeechRecognition`

`LLM_BASE_URL` already points at an OpenAI-compatible litellm proxy, so `/audio/transcriptions`
is the same host and the same key — no new infrastructure, no new secret. A new `STT_MODEL`
(default `whisper-1`) names the model, so a deployment can point it elsewhere.

*Alternative considered:* the browser's `SpeechRecognition`. Free and serverless, but it needs
`lang` chosen in advance, is absent in Firefox, and is materially worse on Russian. Whisper
detects the language itself, which removes a product question rather than answering it.

*Alternative considered:* both, with the browser engine as default and Whisper as fallback —
what open-webui does. Rejected as two recording paths and two result shapes to maintain for a
first release.

### `internal/speech` is a plain `net/http` client, not part of `internal/llm`

`internal/llm` is built on langchaingo, which models chat completions and has no audio surface.
Bolting a multipart audio POST onto it would mean reaching around the abstraction it exists to
provide. A separate package with one method (`Transcribe(ctx, audio []byte, filename string)
(string, error)`) is smaller and honest about what it is. It takes the same `BaseURL` and
`APIKey` values `llm.Settings` does, wired in `cmd/server/main.go` beside them.

A nil client means "not configured", following `internal/handler/photo.go`: the handler answers
`501`, and the SPA reads that as "this surface does not exist here" rather than as a fault.

### The endpoint authenticates like the message endpoint, and is rate limited per caller

The microphone lives in the composer that posts messages. Anything allowed to post a message
should be allowed to transcribe for it, so the route takes the same middleware as
`POST /assistant/sessions/:id/messages` rather than inventing a narrower rule that would make
the microphone silently dead in the extension's side panel.

Spend is bounded by a per-caller limiter (the `photo.go` pattern — keyed on the user, because
an IP key is lifted by any rotating proxy pool), a 2 MiB body cap enforced on bytes read rather
than on the declared size, and a client-side recording ceiling that keeps the cap from being
the common path.

### Dictation appends to the draft; the caller sends

A transcription is a guess. Auto-sending a wrong guess produces a message that cannot be
unsent, and — because the agent has tools — a message that may act. Appending to the draft
keeps the human in the loop at zero cost to the flow.

### Follow-ups are a separate endpoint on the cheap model, not part of the turn

Generating them inside the turn loop would make a failure to suggest a failure to answer, and
would spend the assistant's large-context tool-calling model on a three-line task. A separate
`POST /assistant/sessions/:id/followups` runs `LLM_MODEL` under `llm.WithSchema`, sees only the
most recent exchange, and returns `{"data": {"followups": [...]}}`. Every failure path returns
an empty list rather than an error — the strip is decoration, and decoration must not be able
to report a problem the caller cannot act on.

Server-side caps: at most three items, each at most a fixed length, with over-length items
discarded rather than truncated (a truncated question is a different question).

### Follow-ups render as text nodes

Not through `renderMarkdown`. The suggestion becomes the caller's own message on click, so it
must be legible exactly as it will be sent, and must not be able to style or hide part of
itself. This is the same boundary `assistant-output-rendering` draws, drawn tighter because the
consequence is stronger.

### Scroll following is one boolean, updated from the pane's own scroll events

`stickToBottom` is set in an `onscroll` handler by `scrollHeight - scrollTop <= clientHeight +
TOLERANCE`. `scrollToBottom()` becomes a no-op while it is false; a `force` argument covers the
deliberate acts (send, run, open session) that must return to the bottom regardless.

The tolerance exists because the final line of a streaming answer grows underneath the reader:
an exact equality test drops out of following on its own, on its own content.

*Alternative considered:* an `IntersectionObserver` on a bottom sentinel. Cleaner in principle,
but it fires asynchronously and would let one or two frames land before following resumes,
which is visible as a stutter at exactly the moment the reader returns to the bottom.

## Risks / Trade-offs

- **Unmetered transcription cost** → per-caller limiter, 2 MiB cap, client-side recording
  ceiling. Worst case per caller per window is bounded and small; the real fix is metering,
  and the seam is unchanged.
- **One extra model call per settled turn** (~30% more calls, on the cheapest model, with a
  two-message input) → capped input, cheap model, and the call is skipped entirely for
  errored, cancelled and ceiling-stopped turns.
- **A follow-up carrying an injected instruction** → rendered as inert text, length-capped
  server side, and activation is an explicit click on text the caller can read in full.
- **`MediaRecorder` container support varies** (Safari emits mp4, Chrome webm/opus) → probe
  `isTypeSupported` over a preference list and send the container the browser actually
  produced, with a matching extension, since the upstream sniffs by extension and content type.
- **Mobile screen lock stopping a recording mid-sentence** → request a screen wake lock while
  recording and release it on every exit path. The API is absent in some browsers; its absence
  is not an error.
- **A recording left running in a background tab** → the client ceiling stops it; the wake lock
  is released whenever recording ends.

## Migration Plan

No schema change and no migration. `STT_MODEL` is the only new configuration and it has a
default; a deployment without a speech-capable gateway answers `501` and renders no microphone.
Follow-ups need no configuration and degrade to an empty list.

Rollback is removal: no persisted state is written by any of the three features.

## Open Questions

None blocking. Two deferred by choice: whether follow-ups should be restricted to the `chat`
preset (they are offered everywhere for now, and the prompt is preset-agnostic), and whether
dictation should eventually stream partial results instead of transcribing on stop.
