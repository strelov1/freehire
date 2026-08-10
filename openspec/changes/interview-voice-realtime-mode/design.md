## Context

`interview` is one of `internal/assistant`'s five presets: a mock interview run against
one application, with the agent speaking first under a server-built brief
([[hire-interview-rehearsal-preset]] in prior notes). Today every turn is text: the
candidate types (or dictates via `internal/speech`, which only fills the text box — see
`internal/speech/AGENTS.md`), the model streams a text reply over SSE, and
`assistant.Runner.persist` writes each message to the session's transcript
(`internal/assistant/runner.go:345`, backed by `Store.Append`). The model discovers the
vacancy, stage, fit-analysis requirements and evidence, and any employer invitation by
calling the `interview_context` tool on its first turn
(`internal/handler/assistant_interview_tools.go`, `rehearsalContext`) — it is fetched
live, not pre-baked into the system prompt.

A throwaway spike confirmed that a cascaded STT→LLM→TTS pipeline (browser mic → Whisper
via the existing `internal/speech` gateway → text turn → TTS) is not viable for a
hands-free conversation: Whisper's `/audio/transcriptions` is one-shot, non-streaming
(`internal/speech/AGENTS.md` already documents this — "no streaming, no partial
results"), so every turn carries multi-second dead air proportional to how long the
candidate spoke, and no client-side VAD tuning fixes that; it's an upstream API
constraint, not an implementation gap. Reconnecting the same spike to OpenAI's Realtime
API over WebRTC (model `gpt-realtime-2.1`) removed the wait entirely: turn-taking,
interruption, and speech synthesis are handled by the model's own session, and audio
never round-trips through any of our servers.

## Goals / Non-Goals

**Goals:**
- Hands-free spoken rehearsal for an existing `interview` session, indistinguishable in
  outcome from the text mode: same vacancy context, same transcript storage, readable
  the same way whether a candidate switches between voice and typed turns mid-session.
- Keep the audio path itself off our infrastructure — the browser talks to OpenAI
  directly over WebRTC once it holds a scoped, short-lived credential.

**Non-Goals:**
- No tool calls inside a voice turn. `experience_add` and every other `interview` tool
  stay text-only for v1; a voice session can reference what the bank already holds
  (baked into the opening `instructions`, the same way `interview_context` feeds the
  text mode today) but cannot write to it. Extending this needs its own change — it
  means either a client-side tool-call relay to our REST API or an authenticated
  WebSocket relay, both real integration work, not a corollary of shipping voice mode.
- No general-purpose voice mode for the other four presets. Scope is `interview` only;
  a second preset wanting this is a reason to extract a shared component, not a reason
  to build one preemptively now.
- No offline/degraded fallback beyond what already exists: if the Realtime credential
  is unconfigured, the entry point simply does not render, following the
  `internal/speech` nil-client convention.

## Decisions

**Instructions are prefetched, not tool-called.** The token-minting endpoint calls the
same `rehearsalContext`-shaped lookup the text preset's `interview_context` tool uses,
renders it to text once, and passes it as the Realtime session's `instructions` at
creation. The alternative — giving the voice session a live `interview_context` tool
over the data channel — is real Realtime API functionality (client-side JS can answer a
function-call event by hitting our own authenticated REST endpoint), but it is exactly
the kind of tool-relay machinery the Non-Goals section defers, and the context does not
change mid-call, so prefetching loses nothing.

**Turns persist through the existing store, not a new table.** Each completed exchange
(the candidate's input-transcript-completed text, the agent's output-audio-transcript
text) is appended via the same `Store.Append` primitive `Runner.persist` already calls —
a new small handler endpoint takes `{role, content}` for one finished turn and writes it
scoped to the caller's own session, the same ownership check the other `interview`
endpoints use. This keeps the plain transcript view, and any later typed turn in the
same session, unaware of which modality produced a message — the model's context is
built from the stored history regardless of how it got there. (Earlier drafts of this
design also justified persistence by `interview-debrief` reading it afterward; that
was wrong — `debriefPrompt` explicitly works from what the candidate recounts in a
fresh session, "Everything you know about what happened comes from them," and never
reads another session's transcript, voice-driven or not. Persistence is justified on
its own: the session's own history view, and mid-session continuity.) The alternative
(a separate voice-transcript table, reconciled into the session view later) adds a
second source of truth for no benefit — nothing here needs it to be anything other
than a message.

**Credential shape: routed through the existing litellm proxy, not a direct OpenAI
key.** Resolved by reading the litellm source that backs the privatclaw deployment
(`/Users/i_strelov/Projects/litellm/litellm/proxy/realtime_endpoints/endpoints.py`):
it already implements `create_realtime_client_secret` and `proxy_realtime_calls`, the
same GA client-secret-mint + WebRTC-relay flow the spike used directly against OpenAI.
The live proxy's `config.yaml` has no `realtime`-capable model deployment configured
yet (an ops step, outside this change's code), but once one exists, hire's backend
mints the client secret through the existing `LLM_BASE_URL`/`LLM_API_KEY` — no new raw
OpenAI credential, and per-user `internal/llmkey` attribution keeps applying the same
way it does for chat and STT. The new env var this change adds is just
`REALTIME_MODEL` (mirroring `STT_MODEL`: no default, unset means the feature is
absent), naming which model alias to request.

**No manual VAD/barge-in code.** The spike's biggest source of bugs was hand-rolled
silence detection (RMS threshold, re-entrant timers, echo from the TTS output
retriggering the mic). The Realtime API's `semantic_vad` turn detection and built-in
interruption handling replace all of that — there is no client-side turn-taking state
machine to write or to get wrong a second time.

## Risks / Trade-offs

- **[Cost]** Realtime audio bills per minute (~$0.06/min in, ~$0.24/min out per the
  spike's pricing check), not per token — a rehearsal call that runs long is a
  meaningfully different cost shape than a text session of the same length. →
  Mitigation: a client-side call-duration cap (mirroring `internal/speech`'s
  `MAX_RECORDING_MS` client ceiling, task 5.4) bounds one call's length, and a
  server-side per-caller hourly limit on the mint endpoint (`voiceTokensPerHour`,
  mirroring `speech`'s `transcriptionsPerHour`) bounds how many calls a script can
  start — the client cap alone is not a backstop against a caller that skips the UI.
- **[Client-asserted turn role]** `PostAssistantVoiceTurn` accepts `role` from the
  request body and, for `"assistant"`, writes it into the transcript exactly as sent —
  no other write path in this codebase lets a caller author assistant-role content
  (`PostAssistantMessage` only ever accepts user text). → Accepted for v1: the write is
  confined to the caller's own session, voice mode carries no tool that could act on a
  fabricated instruction (see the Non-Goals cut), and the worst case is a candidate
  steering their own rehearsal, not another party's. A tighter design — the server
  assigning role from which WebRTC data-channel event produced the turn, rather than
  trusting the client's label — is worth doing if voice mode ever gains a tool or a
  second session sees the same transcript.
- **[Credential exposure]** A raw OpenAI key must never reach the browser — only the
  minted ephemeral `client_secrets` value (one-minute default TTL) does. → Mitigation:
  token minting is entirely server-side, mirroring how `internal/speech` already never
  hands the gateway key to the client; the spike's own local proxy never exposed the raw
  key to the page, and this is the pattern to keep.
- **[Mid-call drop]** A call can end mid-sentence (network blip, tab close, browser
  crash) with a partial turn that the client never got to persist. → Mitigation: accept
  a possibly-truncated final turn as a known limitation for v1; not worth building
  reconciliation-on-reconnect against a live third-party session for a rehearsal tool.
- **[Vendor coupling]** This wires a specific OpenAI model family (`gpt-realtime-*`)
  directly into the frontend, unlike the rest of the assistant's provider-agnostic
  `internal/llm` (langchaingo) path. → Accepted: no other provider currently ships an
  equivalent speech-to-speech API; documented here so it is a known, not accidental,
  departure from the "provider-agnostic LLM" convention.

## Migration Plan

No schema migration. Deploy is additive: new route(s), new env-gated credential, new
frontend entry point behind the same "does this feature exist here" nil-client check
`internal/speech` already established. Rollback is deleting/unsetting the credential —
the entry point stops rendering, same as speech does today when `STT_MODEL` is unset.

## Open Questions

- ~~Does the litellm/privatclaw proxy already support proxying the Realtime API~~ —
  resolved, see Decisions: yes, routed through the existing proxy credential. The
  live proxy's `config.yaml` still needs a `realtime`-capable model deployment added
  (ops work, tracked outside this change) before the token-minting handler can succeed
  end-to-end against the real deployment.
- What voice should the interviewer persona use, and does its grammatical gender need to
  be pinned in the instructions (the spike surfaced that the model does not infer this
  from the voice on its own — see [[hire-interview-rehearsal-preset]])?
- ~~Should the call-duration cap be a hard disconnect or a warning-then-disconnect~~ —
  resolved during frontend implementation: warning-then-disconnect (`END_WARNING_MS`,
  one minute's notice before `MAX_CALL_MS` ends the call), because a silent cutoff is
  indistinguishable from a dropped connection to whoever is mid-answer when it fires.
