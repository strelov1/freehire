## 1. Transcript scrolling

- [x] 1.1 Add `web/src/lib/assistant/scrolling.ts` with the pure decision — `atBottom(metrics, tolerance)` and the tolerance constant — plus `scrolling.test.ts` covering at-bottom, scrolled-away, and the growing-last-line case the tolerance exists for.
- [x] 1.2 Wire `AssistantChat.svelte` to it: a `stickToBottom` state updated from the pane's `onscroll`, `scrollToBottom(force = false)` that no-ops while false, and `force` at the three deliberate acts (submit, unattended run, open session).
- [x] 1.3 Add the "jump to latest" control, shown only while not following; activating it scrolls to the bottom and resumes following.
- [x] 1.4 Verify in a browser against a long streaming answer: scroll up mid-turn and confirm position holds, the control appears, and both the control and scrolling back restore following.

## 2. Transcription backend

- [x] 2.1 Create `internal/speech` with `Client`, `New(baseURL, apiKey, model)` returning nil when unconfigured, and `Transcribe(ctx, audio []byte, filename string) (string, error)`; test the multipart body it builds, the `text` it parses, and its error mapping against an `httptest` server.
- [x] 2.2 Add `STT_MODEL` to `internal/config` with default `whisper-1`, covered by the config test.
- [x] 2.3 Add `internal/handler/speech.go`: `POST /api/v1/speech/transcriptions`, the message endpoint's middleware, a per-caller limiter, `readAudioUpload` capping on bytes read, and the error mapping (400 / 401 / 429 / 501 / 502).
- [x] 2.4 Add handler tests for each status in 2.3, including "no upstream call is made" for the refusal paths.
- [x] 2.5 Wire the client in `cmd/server/main.go` beside the existing LLM clients and pass it to the handler.

## 3. Dictation UI

- [x] 3.1 Add `web/src/lib/assistant/dictation.ts` with the pure parts — `appendTranscript(draft, text)`, the container/extension preference list resolved against a supplied `isTypeSupported`, `canRecord(navigatorLike)`, and the recording ceiling — plus `dictation.test.ts` covering empty draft, trailing whitespace, empty transcription, and an unsupported browser.
- [x] 3.2 Add `web/src/lib/assistant/VoiceInput.svelte`: idle / recording / transcribing states, elapsed time, cancel, the ceiling, wake lock acquire-and-release, and track teardown on every exit path.
- [x] 3.3 Add the transcription call to `web/src/lib/assistant/api.ts` and mount `VoiceInput` in `Composer.svelte`, appending its result to `draft`; hide the control where `canRecord` is false or the endpoint reported `501`.
- [x] 3.4 Surface denied permission, capture failure and transcription failure as messages the caller can act on, leaving the composer typable.
- [x] 3.5 Verify in a browser: record, cancel, deny permission, and reach the ceiling.

## 4. Follow-ups backend

- [x] 4.1 Add follow-up generation to `internal/assistant` (prompt + `llm.WithSchema` shape), returning at most three items and discarding over-length ones; unit-test the caps and the discard-not-truncate rule against a fake model.
- [x] 4.2 Add `internal/handler/assistant_followups.go`: `POST /api/v1/assistant/sessions/:id/followups`, owner-scoped, reading only the most recent exchange, answering an empty list on every failure path.
- [x] 4.3 Add handler tests: owned session returns items, another caller's session is 404, no credential is 401, an unconfigured or failing model returns an empty list with 200.
- [x] 4.4 Register the route and wire the cheap model client.

## 5. Follow-ups UI

- [x] 5.1 Add `web/src/lib/assistant/followups.ts` with `shouldRequest(result, text)` and the display truncation, plus tests for `end_turn` with text, errored, cancelled, ceiling-stopped, and empty-text turns.
- [x] 5.2 Request follow-ups from `AssistantChat.svelte` when the reducer settles a qualifying turn; clear them when the next turn starts and never request them on transcript replay.
- [x] 5.3 Render the strip beneath the last assistant message as text nodes only, with a click that goes through `submitText`.
- [x] 5.4 Verify in a browser that the strip appears after an answer, clears on the next turn, and is absent after an error and on reopening a conversation.

## 6. Close out

- [x] 6.1 Update `internal/assistant/AGENTS.md` (the follow-up endpoint and its "failure is invisible" rule) and add `internal/speech/AGENTS.md` plus its row in the root `CLAUDE.md` table.
- [x] 6.2 Document `STT_MODEL` wherever the other LLM environment variables are documented.
- [x] 6.3 Run `go build ./... && go vet ./... && go test ./...` and the web checks; verify against the acceptance scenarios in the three spec files.
- [ ] 6.4 Offer a `/blog` changelog entry for the shipped voice input and follow-ups.
