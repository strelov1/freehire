## Why

A real production tailoring session (`assistant_sessions` id
`bf58de6a-a83b-4056-aa7e-16e3a904a70a`) asked the candidate to "confirm in your own
words" 16 times and never recovered — the transcript's last message is still the same
refusal. Part of the root cause (a dead end in `experience_add`'s dedup, where a claim
once banked as `agent_inferred` could never be upgraded even by a later, genuinely
verbatim confirmation) is already fixed and shipped (`c7901ec9`). What remains is the
UX that produced the sixteen retries in the first place — the agent asking the
candidate to retype a claim verbatim, over and over, in plain chat text — and a
separate, independently-requested removal of the Follow-ups suggestion strip, which the
same session shows firing after nearly every exchange.

## What Changes

- Add a `request_confirmation` tool, registered for the `tailor` preset only, that the
  tailoring agent calls instead of writing a confirmation question as free prose. The
  client renders it as the claim text plus **Да**/**Нет** buttons; **Да** posts the
  claim text verbatim as an ordinary chat message (reusing the existing
  `submitText` path), which satisfies the unchanged verbatim-quote provenance check on
  the agent's next `experience_add` retry without the candidate retyping anything.
  **Нет** posts a fixed decline message.
- **BREAKING**: Remove the Follow-ups feature entirely — the suggested-next-question
  chip strip shown beneath a settled assistant turn, and its endpoint
  (`POST /assistant/sessions/:id/followups`). Not the same-named but unrelated
  application follow-up-email-draft feature in `internal/followup/`, which is untouched.

## Capabilities

### New Capabilities
- `tailor-claim-confirmation`: the tailoring agent requests confirmation of an unbanked
  claim through a dedicated tool and button UI instead of free-text prose, so confirming
  is a click that always produces a verbatim-matching transcript message.

### Modified Capabilities
- `assistant-follow-ups`: every requirement in this capability is removed — the feature
  it specifies no longer exists.

## Impact

- Backend: `internal/handler/assistant_cv_tools.go` (new tool), `internal/assistant/prompt.go`
  (`tailorPrompt` step 2), `internal/handler/assistant_tools.go` (tool registration gate).
  Deleted: `internal/assistant/followups.go` (+ test), `internal/handler/assistant_followups.go`
  (+ unit and integration tests), the route in `internal/handler/assistant.go`, the
  route-list assertion in `internal/handler/assistant_integration_test.go`, the
  `tagFollowUps` billing tag in `internal/handler/user_llm.go` (after confirming nothing
  else references it), and the "Follow-ups" section of `internal/assistant/AGENTS.md`.
- Frontend: `web/src/lib/assistant/ToolGroupList.svelte` (new name-conditional render
  branch), `web/src/lib/assistant/AssistantChat.svelte` (wiring for the new branch's
  buttons; removal of the Follow-ups state, request, and chip-rendering block). Deleted:
  `web/src/lib/assistant/followups.ts` (+ test), the `suggestFollowUps` call in
  `web/src/lib/assistant/api.ts`.
- No schema change, no new SSE event type, no new REST endpoint.
