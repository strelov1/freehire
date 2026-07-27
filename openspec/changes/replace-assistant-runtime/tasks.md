## 1. LLM tool-calling foundation

- [x] 1.1 Add a conversational generate method to `internal/llm`: takes `[]llms.MessageContent`, a `[]llms.Tool` list and a token-delta callback, returns the full `*llms.ContentChoice` (text, `ToolCalls`, usage); reuses the existing timeout, tracer and usage extraction. `GenerateJSON`/`GenerateJSONStream` signatures stay untouched.
- [x] 1.2 Cover it with unit tests against a fake `llms.Model`: a tool-call response, a plain-text response, an empty-choices guard, and a timeout.

## 2. Session and transcript persistence

- [x] 2.1 Add migration `00XX_assistant_sessions.sql`: `assistant_sessions` (id, user_id, preset, label, cv_id, job_id, created_at, updated_at) and `assistant_messages` (session_id, seq, role, content jsonb, created_at) with owner and ordering indexes and cascade delete.
- [x] 2.2 Add sqlc queries for create/list/get/delete session (all owner-scoped) and append/read messages; regenerate `internal/db`.
- [x] 2.3 Add a store in `internal/assistant` over those queries that maps stored messages to and from `llms.MessageContent`, including assistant tool calls and tool results; unit-test the round trip.

## 3. Tool registry and tools

- [x] 3.1 ~~Extract the facet-filter construction out of `buildSearchFilter`~~ — not needed. `buildSearchFilter` already delegates to `search.FilterFromValues(url.Values)`, which is fiber-free, so the tool renders its typed arguments as `url.Values` and reuses it. No refactor; design.md updated to match.
- [x] 3.2 Define the tool contract in `internal/assistant`: a tool is a name + description + JSON schema + `func(ctx, userID, json.RawMessage) (any, error)`; add a registry that renders `[]llms.Tool` and dispatches a call by name, decoding arguments strictly and returning decode/exec errors as tool results.
- [x] 3.3 Implement the discovery tools — `facets`, `search_jobs` (full descriptions, `public_slug` present), `get_job`, `get_company`, `market_fit` — over the same services the handlers use.
- [x] 3.4 Implement the tracking tools — `save_job`, `unsave_job`, `apply_job`, `track_job`, `my_jobs` — over `internal/userjob`/`internal/jobtracking`, owner-scoped.
- [x] 3.5 Implement the CV tools — `cv_context`, `cv_get`, `cv_edit` — over `internal/cv`, reusing `cv.DecodePatch` and rejecting contact-header edits as the HTTP path does. The ids are closed over from the session binding, not taken as arguments. `cv_render` dropped: the workspace previews the CV beside the chat, so a render tool returns bytes the model cannot read (spec updated).
- [x] 3.6 Add the result-size cap: truncate an oversized tool result with an explicit marker before it enters history; unit-test the boundary.

## 4. Agent loop

- [ ] 4.1 Implement the turn loop in `internal/assistant`: model call with tools → execute tool calls → append results → repeat, stopping on a text answer or the configured step cap, and making one final tool-less call when the cap is hit.
- [ ] 4.2 Emit turn events from the loop (`user_prompt`, `assistant_text`, `assistant_thought`, `tool_use`, `tool_result`, `usage`, `result`) through a callback, and persist each to the transcript as it is produced.
- [ ] 4.3 Honour context cancellation: stop before the next model call, persist the partial transcript, end with a cancelled stop reason.
- [ ] 4.4 Bound the replayed history to the most recent N messages when composing a turn.
- [ ] 4.5 Unit-test the loop with a scripted fake model: tool-then-answer, cap reached, malformed tool arguments corrected in-turn, tool error surfaced to the model, cancellation mid-turn.

## 5. Presets and prompts

- [ ] 5.1 Define the two presets: `chat` (discovery + tracking tools) and `tailor` (those plus CV tools, bound to a CV id and job id), each selecting its system prompt.
- [ ] 5.2 Write the system prompts, carrying the `using-freehire` playbook: read `facets` before filtering, canonical skill slugs, present vacancies as `/jobs/<public_slug>` one per line, and the tailoring honesty rules (`missing_have` reframe vs `missing_gap` ask-first).
- [ ] 5.3 Assert in tests that a chat session offers no CV tool and that no moderator tool is ever registered.

## 6. HTTP surface

- [ ] 6.1 Add the session endpoints under `/api/v1/assistant` — create, list, get transcript, delete — cookie-authenticated, owner-scoped, behind the beta-tester gate.
- [ ] 6.2 Add the turn endpoint: `POST /api/v1/assistant/sessions/:id/messages` streaming named SSE events with a keep-alive comment, following `match_analysis_stream.go`.
- [ ] 6.3 Add integration tests: full turn stream with a fake model, transcript persisted, owner checks on every endpoint (list/read/delete/turn), beta gate, and the slug-addressed `get_job`/`get_company` tools (query-backed, so covered here rather than by unit tests).
- [ ] 6.4 Add config for the assistant's model, per-call timeout and step cap; document them in the config comment and `CLAUDE.md`.

## 7. Tailoring bootstrap

- [ ] 7.1 Remove `mintTailoringKey` and the `cli_token` field from `TailorCV` and `StartTailorSession`; update their tests. The `cv` API-key scope itself stays for the public CLI.
- [ ] 7.2 Have the tailoring bootstrap create a `tailor`-preset assistant session bound to the tailored CV and its vacancy, and return its id where the workspace expects a session id.

## 8. Frontend transport

- [ ] 8.1 Rewrite `web/src/lib/assistant/wire.ts` as the new turn-event union (drop `ClientCommand`/`ServerEvent`, `system`, `note`, `raw`; add `tool_result`).
- [ ] 8.2 Replace `client.ts`'s `RoyClient` with an SSE turn client: send a message, stream events, cancel; keep the same callback shape `AssistantChat.svelte` consumes.
- [ ] 8.3 Point `api.ts` at `/api/v1/assistant` (same-origin, `credentials: 'include'`), drop `assistantWsUrl`, `NoDeviceError`, `runnerStatus` and the tailoring `cli_token` field.
- [ ] 8.4 Update `chat.ts`'s reducer for the new union and add `tool_result` handling; keep its existing tests green and extend them.
- [ ] 8.5 Trim `tool-formatters.ts` of the shell branches (`bashCommand`, `isNoiseShellCall`, `isFreehireGroup`, `commandLine`) and format tool cards from tool name + typed arguments; update its tests.

## 9. Frontend cleanup

- [ ] 9.1 Delete `RunnerSetup.svelte`, `RunnerBadge.svelte` and every runner branch in `AssistantChat.svelte` and `/tailor/[slug]/+page.svelte`, keeping the rest of the markup and behaviour identical.
- [ ] 9.2 Remove the `/assistant-api` Vite proxy and the `PUBLIC_ASSISTANT_ORIGIN` reference from the web build.
- [ ] 9.3 Verify both surfaces in a browser: `/my/assistant` (send, stream, tool cards, job cards, session rail, switch, delete) and `/tailor/<slug>` (kickoff, CV edit through the agent, preview refresh on turn complete).

## 10. Documentation

- [ ] 10.1 Write `internal/assistant/AGENTS.md` covering the loop, the tool contract, the presets and the persistence model; link it from `CLAUDE.md`'s module table.
- [ ] 10.2 Update `internal/handler/AGENTS.md` with the new routes, and note in the ops repo's `docs/agent-deploy.md` that the vhost and units are retired once this ships.
