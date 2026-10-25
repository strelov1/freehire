# Auto-apply chat — design

Date: 2026-10-25. Status: approved in conversation, awaiting spec review.

## Problem

An automatic application stops on questions the candidate must answer (`blocked`), and
waits for their decision before it is sent (`pending_review`). Today the only web surface
for both is the job drawer's auto-apply tab: one textarea and one Save button per question,
then a separate Try again / Approve button. A Workable form with fourteen questions is
fourteen boxes. The candidate also cannot say "same as last time, but 6000 instead of 5000"
— every answer is a form field.

Over MCP the same work is conversational: the agent reads the open questions, asks the
person, saves what they said and retries (`auto_apply_get` / `answer_bank_save` /
`auto_apply_retry` / `auto_apply_review`). The in-app assistant has none of those tools —
`autoApplyMCPTools` is registered on the MCP account server only, on the reasoning that the
browser "already has the job page's button and the drawer's review".

## Decisions (from the design conversation)

1. **One chat per application**, opened from the job drawer's auto-apply tab. Not one
   shared chat for every application: questions belong to one form, and "answer 3" must
   mean one attempt.
2. **Everything about the attempt happens in the chat**: answering questions, reviewing
   what will be sent ("send it" / "don't" / "make the salary 6000"), cancelling. The
   drawer's existing buttons stay as the fallback, not the primary path.
3. **The model writes every message, status updates included**, in the candidate's own
   language (the site locale). Chosen over deterministic status rows because the messages
   read naturally in any language without a hand-written string per status.
4. **Status messages are written when the chat is open**, not in the background for every
   step of every attempt: the bell notification (`auto_apply_ready_for_review`, the
   blocked/failed/submitted nudges) is what calls the candidate in; the chat catches up when
   they arrive.
5. **The candidate answers in their language; the employer receives the form's.** The model
   translates an answer into the form's language before saving it, and the review step
   shows the translated text before anything is sent.
6. **MCP keeps its existing path**, with its guidance aligned so an external agent also asks
   all open questions as one numbered list.

## Design

### The session

A new assistant preset, `apply`, bound to one application exactly the way `interview` and
`debrief` are (`bindsToApplication`, `applicationVacancy` in
`internal/api/handler/assistant.go`): created with `?preset=apply&job=<slug>`, the
`user_jobs` row is the authorisation, `assistant_sessions.job_id` holds the vacancy. One
live session per (user, job); reopening the drawer resumes it rather than starting another.
The preset needs a migration widening the `assistant_sessions.preset` CHECK, the same way
0048/0049/0062/0069 did.

### The tools

The `apply` preset registers the auto-apply tools `autoApplyMCPTools` already defines, bound
to the session's job: `auto_apply_get`, `auto_apply_review`, `auto_apply_retry`,
`auto_apply_cancel`, `answer_bank_save`, `answer_bank_list`, `screening_answers_get` /
`screening_answers_set`. Not `auto_apply_start` or `auto_apply_list` — the session is one
attempt that already exists.

- **Scoped to the session's job.** The tools take no slug from the model; the session
  supplies it. A model cannot act on another application from this chat.
- **No `agents_may_apply` gate here.** That switch exists because a connected agent acts
  outside the person's browser. This chat is the person's own browser session talking to
  their own application, the same authority as the drawer's buttons.
- **The existing tool rules hold unchanged**: an answer is the person's, never composed by
  the model; review is one decision per attempt; an attempt that may already have reached the
  employer is never retried or cancelled.

### The catch-up turn

When the chat opens, and every ~15 s while it stays open, the client asks
`POST /assistant/sessions/:id/sync`. The server compares the attempt's `auto_apply_events`
against the last event the session has already narrated (a cursor on the session). If
there is something new, it runs one turn whose input is a system note listing those events
plus the current `auto_apply_get` view; the model writes one message about them — for
`blocked`, the open questions as a single numbered list. Nothing new, no turn and no cost.
The cursor advances only after the turn is stored, so a failed turn is retried on the next
sync instead of being skipped.

This is the one new kind of turn: the input is not a user message. The note is stored with
`role = system` (the transcript CHECK already allows it; nothing writes it today) and is
not rendered in the chat.

### The candidate's reply

An ordinary turn. The model matches "1 Brazil, 3 five years …" against the numbered list it
posted, asks back about anything it cannot place, translates each answer into the form's
language, saves it (`answer_bank_save`, or `screening_answers_set` for the six screening
facts), and calls `auto_apply_retry` once every open question has an answer. A reply that is
a review decision ("yes, send it") calls `auto_apply_review`; a correction at review ("make
the salary 6000") saves the new answer, retries to refresh the preview, and asks again.

### Language

The system prompt carries the site locale; every message the model writes is in it. Answers
saved for the employer are in the form's language, and the review message quotes them as
they will be sent.

### Metering

Catch-up turns are written for the candidate, not asked for by them, so they do not draw on
the daily assistant-message allowance; the candidate's own replies do, like any other turn.
A catch-up turn is bounded by the attempt's events (a handful per attempt) and runs only
while the chat is open. Auto-apply itself already requires a paid plan.

### Web

The chat opens in the docked assistant panel the experience bank already uses
(`web/src/lib/components/ExperienceAssistantPanel.svelte`), not a new surface. That panel
already solves what this needs: it docks beside the page instead of covering it where there
is room, mounts the chat (and so creates the session) only on first open, keeps it mounted
while hidden so a streaming turn is not cancelled, and links out to the full assistant. It is
generalised into one `AssistantPanel` taking the session's preset and binding (`profile` +
atoms for the bank, `apply` + job slug here); the experience bank keeps its behaviour.

The job drawer's auto-apply tab gets a "Answer in chat" action that opens the panel on the
attempt's `apply` session, shown whenever the attempt is `blocked` or `pending_review`. The
per-question textareas stay in the tab as the form alternative. `AssistantChat` gains the
periodic sync while mounted and bound to an `apply` session.

### MCP

`autoApplyMCPTools` already exposes everything. The change is guidance only: the server
instructions and `auto_apply_get`'s `next_step` ask the agent to put every open question to
the person as one numbered list, and to translate answers into the form's language.

## Out of scope

- Background narration of attempts nobody is looking at.
- A shared "all my applications" chat.
- Push to the open chat (server-sent status); polling every ~15 s while open is enough for
  a worker that runs every five minutes.

## Testing

- Handler: an `apply` session is refused for a job the user does not hold; its tools refuse
  a slug (none is accepted) and act on the session's job only; no `agents_may_apply` check.
- Sync: no new events → no turn, cursor unchanged; new events → one turn, cursor advanced;
  a failed turn leaves the cursor, so the next sync retries it.
- Metering: a catch-up turn does not consume the assistant-message allowance; a reply does.
- Prompt-level behaviour (numbered list, translation, asking back) verified by a live run on
  production against a real blocked attempt, as the MCP path was.
