## Context

The approved design, with the reasoning behind each product decision, is
`docs/superpowers/specs/2026-10-25-auto-apply-chat-design.md`. This file records the
technical decisions for building it.

Today: assistant sessions (`assistant_sessions`, `assistant_messages`) with presets in
`internal/ai/assistant/store.go`; job-bound presets `interview`/`debrief` minted through
`bindsToApplication`/`applicationVacancy` (`internal/api/handler/assistant.go`); tools are
built per session (`assistant.Session` carries `JobID`); the auto-apply tools exist in
`internal/api/handler/assistant_auto_apply_tools.go` as `autoApplyMCPTools`, registered only
on the MCP account server; `auto_apply_events` is the attempt's ordered history.

## Goals / Non-Goals

**Goals:** one conversation per application that narrates progress and takes free-text
answers and decisions; reuse the existing tools, session binding and docked panel.

**Non-Goals:** background narration of attempts nobody is viewing; a shared chat for all
applications; pushing status to an open chat (polling is enough for a 5-minute worker).

## Decisions

- **Reuse the MCP tool definitions, bound to the session.** The `apply` tools wrap the same
  handlers as `autoApplyMCPTools`, with the slug taken from the session's job instead of the
  model's arguments, and without `requireAgentsMayApply`. Alternative — a second hand-written
  tool set — rejected: two copies of the safety rules drift.
- **Catch-up as a system-note turn.** The sync endpoint builds a note from the unseen events
  plus the `auto_apply_get` view and runs a turn with it as input, stored `role = system`
  (already allowed by the transcript CHECK) and not rendered. Alternative — a synthetic user
  message — rejected: it would show in the transcript and meter like a user turn.
- **Cursor on the session.** A nullable `narrated_event_id` on `assistant_sessions`, advanced
  in the same transaction that stores the turn. Alternative — derive from the transcript —
  rejected: notes are free text.
- **Metering.** Catch-up turns skip the assistant-message allowance; they are bounded by the
  attempt's event count and only run while the chat is open.
- **Locale.** The session's system prompt carries the site locale passed at session creation.
- **Web.** `ExperienceAssistantPanel` becomes `AssistantPanel` taking preset/binding/kickoff;
  the experience bank keeps its props and behaviour.

## Risks / Trade-offs

- [Model mis-assigns "3 …" to the wrong question] → the numbered list is in the transcript the
  model reads; the review step quotes every answer before sending.
- [Translation changes meaning] → the review message quotes the translated text; nothing is
  sent without the candidate's yes.
- [Sync polling cost] → no new events means no turn; polling stops when the panel closes.
- [Concurrent syncs narrate twice] → the cursor advance is conditional on its old value; the
  losing turn is discarded.

## Migration Plan

One migration (preset CHECK + cursor column), metadata-only. Rollback: the web action is the
only entry point; removing it leaves the drawer's form path as today.

## Open Questions

None.
