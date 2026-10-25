## 1. Session and storage

- [x] 1.1 Migration: widen `assistant_sessions.preset` CHECK with `apply`; add nullable `narrated_event_id bigint`
- [x] 1.2 `internal/ai/assistant`: add `PresetApply` with its system prompt (narrate, one numbered list, site locale, translate answers to the form's language, never compose an answer)
- [x] 1.3 Handler: `apply` binds to an application like interview/debrief; reopening returns the live session for (user, job); refuse a job the user does not hold

## 2. Tools

- [x] 2.1 Register session-scoped auto-apply tools for `apply` (get, review, retry, cancel, answer_bank_save/list, screening_answers_get/set) reusing the MCP handlers, job from the session, no `agents_may_apply` gate
- [x] 2.2 Tests: tools take no slug, act only on the session's job, no switch check; `chat`/`profile` sessions do not list them

## 3. Catch-up sync

- [x] 3.1 Runner support for a turn whose input is a stored, unrendered `system` note
- [x] 3.2 `POST /assistant/sessions/:id/sync`: unseen `auto_apply_events` → one turn; cursor advanced conditionally after the turn is stored; no events → no turn
- [x] 3.3 Catch-up turns skip the assistant-message allowance
- [x] 3.4 Tests: nothing new / new events / failed turn keeps cursor / not metered

## 4. Web

- [x] 4.1 Generalise `ExperienceAssistantPanel` into `AssistantPanel` (preset, binding, kickoff); experience bank unchanged
- [x] 4.2 `AssistantChat`: call sync on open and every ~15 s while mounted on an `apply` session; hide `system` notes
- [x] 4.3 `JobDrawer` auto-apply tab: "Answer in chat" for `blocked`/`pending_review`, opening the panel; keep the form
- [x] 4.4 Client API + i18n (en, ru) for the new action and sync

## 5. MCP and docs

- [x] 5.1 MCP instructions and `auto_apply_get` `next_step`: one numbered list, translate to the form's language
- [x] 5.2 API docs for the `apply` preset and the sync endpoint; regenerate docs
- [x] 5.3 Package AGENTS.md notes (assistant, handler) for the new preset and the system-note turn

## 6. Verify

- [ ] 6.1 Live run on production against a real blocked attempt: open chat, answer in Russian, review, send
