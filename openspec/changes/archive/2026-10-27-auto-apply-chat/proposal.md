## Why

An automatic application that stops on questions (`blocked`) or waits for the candidate's
decision (`pending_review`) can only be handled in the job drawer, one textarea per question
and a separate button to retry or approve. A Workable form with fourteen questions is
fourteen boxes, and "same as last time but 6000" cannot be said at all. Over MCP the same
work is already a conversation; the site should offer it too. Approved design:
`docs/superpowers/specs/2026-10-25-auto-apply-chat-design.md`.

## What Changes

- New assistant preset `apply`, bound to one application (like `interview`/`debrief`).
- The `apply` session gets the auto-apply and answer tools, scoped to its own job, without
  the `agents_may_apply` gate that only connected agents need.
- New `POST /assistant/sessions/:id/sync`: when the attempt has events the session has not
  narrated, runs one model turn over them (stored as a hidden `system` note), not metered
  against the assistant-message allowance.
- The model writes in the site locale and translates answers into the form's language.
- `ExperienceAssistantPanel` generalised into a shared docked `AssistantPanel`; the job
  drawer's auto-apply tab opens it on the attempt's `apply` session.
- MCP guidance asks external agents for one numbered list of open questions.

## Capabilities

### New Capabilities
- `auto-apply-chat`: a per-application assistant conversation that narrates an automatic
  application's progress and takes the candidate's answers and decisions in free text.

### Modified Capabilities

## Impact

- Migration: `assistant_sessions.preset` CHECK gains `apply`; a narrated-event cursor
  column on `assistant_sessions`.
- Go: `internal/ai/assistant` (preset, system-note turn), `internal/api/handler`
  (session creation, tool registry, sync endpoint, metering), `internal/api/mcpapp`
  (instructions).
- Web: `web/src/lib/components/ExperienceAssistantPanel.svelte` → `AssistantPanel`,
  `JobDrawer.svelte`, `web/src/lib/assistant/AssistantChat.svelte` (periodic sync).
- API docs (`web/src/lib/docs/api-spec.ts`) for the new endpoint and preset.
