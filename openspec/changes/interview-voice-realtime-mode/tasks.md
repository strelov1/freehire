## 1. Resolve the credential shape (open question from design.md)

- [x] 1.1 Determine whether the litellm/privatclaw proxy can front OpenAI's Realtime
      API (`/v1/realtime/client_secrets` mint + the WebRTC SDP exchange) under the
      existing per-user `internal/llmkey` attribution, or whether this requires a
      direct, separately-tracked OpenAI credential. Record the answer in design.md's
      Open Questions before proceeding.
      Resolved: yes — litellm already implements both endpoints (verified by reading
      `litellm/proxy/realtime_endpoints/endpoints.py` in the deployment's source repo).
      Route through the existing `LLM_BASE_URL`/`LLM_API_KEY`; no new raw OpenAI key.
- [x] 1.2 Based on 1.1, decide the env var(s) that gate the feature (mirroring
      `STT_MODEL`'s "no default, unset = feature absent" convention from
      `internal/speech`) and where they're read from.
      Resolved: one new var, `REALTIME_MODEL` (no default), read alongside
      `LLM_BASE_URL`/`LLM_API_KEY`/`STT_MODEL` in `internal/config`.

## 2. Backend: Realtime credential client

- [x] 2.1 Add a small client (new package or extend `internal/speech`, per 1.1's
      answer) with a `New(...)` constructor that returns nil when unconfigured,
      following `internal/speech.New`'s pattern.
      Done: new package `internal/realtime`, `New(baseURL, apiKey, model string) *Client`.
- [x] 2.2 Implement the method that mints a Realtime `client_secrets` value scoped to
      one session's instructions text, wrapping upstream failures the way
      `internal/speech`'s `ErrUpstream` does.
      Done: `Client.MintClientSecret(ctx, instructions) (string, error)`, tests in
      `internal/realtime/realtime_test.go` (7 cases, code-reviewed, no Critical/Important
      findings).

## 3. Backend: token-minting endpoint

- [x] 3.1 Build the instructions text for one session by reusing the same lookup
      `rehearsalContext` uses (vacancy, stage, requirements with evidence, employer
      invitation) — one-shot fetch, not a live tool.
      Done: `voiceInterviewInstructions` in `assistant_interview_voice.go`.
- [x] 3.2 Add the minting handler: resolves the session through the caller's own
      ownership (missing, not forbidden, for a session they don't own — matching
      `interview_context`'s existing check), refuses a non-`interview` preset session,
      returns 501 when the credential is unconfigured.
      Done: `PostAssistantVoiceToken`, plus a per-caller hourly rate limit
      (`voiceTokensPerHour`, reusing the renamed `perCallerLimiter`).
- [x] 3.3 Wire the route under the existing `interview` preset endpoint group.
      Done: `POST /assistant/sessions/:id/voice-token`; config/main.go wiring for
      `REALTIME_MODEL` added ahead of schedule since the route is unreachable without it.

## 4. Backend: turn persistence

- [x] 4.1 Add a handler that appends one completed turn (`{role, content}`) to a
      session's transcript via the same `Store.Append` primitive
      `Runner.persist` uses, scoped to the caller's own session.
      Done: `PostAssistantVoiceTurn` at `POST /assistant/sessions/:id/voice-turns`,
      with the same length ceiling `PostAssistantMessage` holds a typed turn to.
- [x] 4.2 Confirm (with a test) that a session assembled from mixed text and
      voice-appended turns replays correctly: the plain transcript view reads both
      kinds of turn in order, and a later typed message's model context includes the
      earlier voice turns. (NOT interview-debrief — corrected during implementation:
      debrief never reads another session's transcript, see design.md's Decisions.)

## 5. Frontend: voice mode entry point

- [x] 5.1 Add a "Voice mode" control to the `interview` session view, rendered only
      when the backend reports the credential is configured (extend whatever signal
      the existing dictation button already uses, or add a matching one).
      Done: gated on `activePreset === 'interview' && voiceSupported && !voiceModeOff`
      in `AssistantChat.svelte`; `voiceSupported` is `canUseVoiceCall` (browser
      RTCPeerConnection/getUserMedia check, mirroring dictation's `canRecord`),
      `voiceModeOff` latches on the backend's 501.
- [x] 5.2 Implement the WebRTC connection: fetch the minted client secret, establish
      the peer connection and data channel per the validated spike flow, attach the
      remote audio track for playback.
      Done: `VoiceCall.svelte`. Also carries `onconnectionstatechange`/data-channel
      `close` handling the spike didn't need (added in review — a mid-call drop must
      not leave the UI stuck showing "active" with the mic still open).
- [x] 5.3 Handle data-channel transcript events (`conversation.item.input_audio_transcription.*`,
      `response.output_audio_transcript.*`) and, on each completed turn, POST it to the
      turn-persistence endpoint from 4.1.
      Done: pure reducer in `voiceCall.ts` (`applyRealtimeEvent`) + `persistTurn` in
      `VoiceCall.svelte`, which retries once before giving up on one turn (a
      completed turn surviving a transient blip is stronger than the spec's bar,
      which only requires surviving a full call drop).
- [x] 5.4 Add a client-side call-duration cap (mirroring `internal/speech`'s
      `MAX_RECORDING_MS` ceiling) with a warning before disconnect; resolve the
      hard-disconnect-vs-warning question from design.md's Open Questions first.
      Done: `MAX_CALL_MS` (15 min) + `END_WARNING_MS` (1 min notice) in `voiceCall.ts`;
      resolved warning-then-disconnect in design.md.
- [x] 5.5 Handle mic-permission denial and connection failure with the same toast
      pattern the existing dictation UI uses.
      Done: inline error state (`phase === 'error'`), which is what dictation
      actually does too — `Composer.svelte` explicitly avoids a toast system for
      voice errors ("legible without a toast system"), so this matches the real
      existing pattern rather than the task text's literal wording.

## 6. Tests

- [x] 6.1 Backend: nil-credential 501, ownership 404, wrong-preset refusal, and
      successful mint shape for the token-minting endpoint.
      Done: `assistant_interview_voice_integration_test.go`, real Postgres via
      testcontainers, `go test -tags=integration ./...` full-module clean.
- [x] 6.2 Backend: turn-persistence endpoint ownership check and transcript ordering.
      Done: same file — ownership, wrong-preset, unknown-role, over-length, plus the
      mixed voice/text replay test (4.2).
- [x] 6.3 Frontend: vitest coverage for the connection/transcript-handling logic that
      doesn't require real microphone/speaker hardware.
      Done: `voiceCall.test.ts` (11 cases: capability check, interleaving, both
      empty-turn-suppression paths, pass-through). Full suite (875 tests),
      `svelte-check` (0 errors), and `vite build` all clean.
- [ ] 6.4 Manual verification in a real browser: start a voice-mode call, confirm
      audio in/out, confirm interruption works, confirm the transcript appears in the
      session afterward. (Not "a debrief session can read it" — corrected per 4.2:
      debrief never reads another session's transcript.)
      BLOCKED: needs a Realtime-capable model deployment on the actual litellm/
      privatclaw proxy, which design.md's Decisions section notes is an ops step
      outside this change's code (the proxy's `config.yaml` has no `realtime` model
      entry yet — verified during task 1.1). Cannot be completed from a worktree with
      no access to that deployment. Do this manually once REALTIME_MODEL is
      configured on a real deployment: open an `interview` session, start a call,
      confirm two-way audio and mid-sentence interruption, end the call, confirm the
      transcript appended, and confirm the entry point disappears if the credential
      is then unset.

## 7. Docs

- [x] 7.1 Update `internal/assistant/AGENTS.md` (or add a note where it links out) to
      mention the voice-mode entry point and the no-tool-calls-in-voice constraint.
      Done: new "Voice mode" subsection between Interview rehearsal and Interview
      debrief.
- [ ] 7.2 Offer a changelog entry per `AGENTS.md`'s "Announce shipped work" convention
      once this lands.
