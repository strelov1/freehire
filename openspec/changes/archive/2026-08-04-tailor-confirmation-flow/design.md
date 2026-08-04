## Context

`internal/assistant/AGENTS.md` and `openspec/specs/cv-tailoring/spec.md` ("Any operation
that asserts something about the candidate must cite evidence") already establish the
invariant this design must not weaken: a claim can only be written into a CV if it cites a
banked achievement whose provenance is something the candidate said (`cv_import`,
`stated_in_chat`, `manual`), never something the model inferred (`agent_inferred`). The
check for `stated_in_chat` is a literal, lowercased, whitespace-collapsed substring match
of the tool's `said` argument against the session's own `RoleUser` transcript messages
(`internal/assistant/message.go`'s `UserSaid`, wired through `provenanceFor` in
`internal/handler/assistant_experience_tools.go`).

Before `c7901ec9`, that check had a dead end: once a claim was first banked as
`agent_inferred`, `experience_add`'s `ON CONFLICT (user_id, claim_key) DO NOTHING` meant no
later call — however genuinely verbatim its `said` — could ever upgrade it. That's fixed.
What's left is that the only way to *produce* a transcript message containing a verbatim
quote was for the candidate to type or paste one back by hand, which is what drove a real
session to ask "confirm in your own words" sixteen times.

Separately, the same transcript shows `POST /assistant/sessions/:id/followups` firing
after nearly every exchange — a feature (`openspec/specs/assistant-follow-ups/spec.md`)
the project owner wants removed outright, unrelated to the confirmation bug.

## Goals / Non-Goals

**Goals:**
- Let the candidate confirm a claim with one click, producing a transcript message that
  satisfies the *existing, unchanged* verbatim-quote check — not a new, weaker path to
  `stated_in_chat`.
- Remove the Follow-ups feature (backend endpoint, frontend chips, and every reference)
  without touching the unrelated, same-named application follow-up-email-draft feature in
  `internal/followup/`.

**Non-Goals:**
- Changing `provenanceFor`, `UserSaid`, or any other part of the evidence gate itself.
- A new REST endpoint or a new provenance category for "confirmed via UI click." The
  design deliberately produces an ordinary chat message instead, so no new trust
  boundary is introduced.
- The larger JD-adaptiveness rework (borrowing reframing/keyword-coverage techniques from
  other tailoring tools) — a separate design pass, out of scope here.

## Decisions

**A new tool, not a new REST endpoint, for the confirm action.** Two shapes were
considered: (1) a `POST /me/experience/atoms/:id/confirm` endpoint that flips provenance
directly from `agent_inferred` to a new "confirmed via UI" value, called straight from the
client; (2) a `request_confirmation` tool the model calls, rendered as buttons that, on
"Да", replay the claim text through the *existing* `submitText` → chat-message → next
`experience_add` retry path. (2) was chosen: it adds zero new surface (no new provenance
value, no new endpoint, no new authorization path) and composes directly with the
already-shipped upgrade-on-conflict fix — the button is a UI shortcut to the same thing a
candidate typing the exact claim text already does today, not a separate mechanism that
has to be independently trusted. (1) would have worked too, but "confirmed via UI click
with no re-read of the claim" is a weaker assertion than "the exact claim text now sits
in the transcript, verbatim, and the standard check evaluates it" — and it would have
meant maintaining two separate ways for a claim to become citable instead of one.

**The tool is a no-op; all the work happens client-side and through ordinary chat
messages.** `request_confirmation({claim, question})` returns
`{"status": "awaiting_candidate_response"}` and touches no service. This keeps the
`tool_use`/`tool_result` SSE contract completely unchanged — the frontend already renders
every tool call generically (`ToolGroupList.svelte`); this adds one name-conditional
branch there, not a new event type. **Да** calls the same `submitText(raw: string)`
(`AssistantChat.svelte:545`) the (now-removed) follow-up chips used — dispatching a
message is not new machinery, only a new caller of it.

**The model still composes the claim text, unchanged.** `request_confirmation`'s `claim`
argument is written by the model exactly as free-text confirmation prose was before; what
changes is the container (a tool call instead of a markdown paragraph) and, downstream,
that the client can echo it verbatim on click instead of the candidate retyping it.

**Follow-ups is deleted, not flagged off.** It has no persisted state (each request is
served fresh from `LastExchange`), so there is nothing to migrate or backfill — deleting
the code paths removes the feature completely with no residue.

## Risks / Trade-offs

- **Risk:** the model, given the new tool, still writes free-text confirmation prose
  instead of calling `request_confirmation` (prompts are advisory, not enforced). →
  **Mitigation:** this is a pure UX regression to today's behavior (the candidate would
  still have to retype), not a correctness regression — the already-shipped provenance
  fix still lets a typed retry succeed. No test can fully pin down model tool-choice
  behavior; the prompt instruction is the primary lever, same as every other tool in
  `tailorPrompt`.
- **Risk:** "Нет" needs the model to actually leave the claim out, per the existing
  prompt rule ("If they say no, leave it out"). → **Mitigation:** unchanged behavior,
  already covered by the existing prompt rule; no new mechanism needed.
- **Risk:** deleting `tagFollowUps` in `internal/handler/user_llm.go` without checking
  for other references could leave a dangling billing tag reference. → **Mitigation:**
  grep for `tagFollowUps` across the module before deleting; tasks.md makes this an
  explicit step, not an assumption.

## Migration Plan

No data migration. Deploy as a normal release: the new tool and Follow-ups removal ship
together since both touch `AssistantChat.svelte` and are trivially small individually.
Rollback is a normal revert — no stored state depends on either change.

## Open Questions

None — the design was fully scoped and agreed with the project owner before this
artifact was written; see `docs/superpowers/specs/2026-08-03-tailor-confirmation-flow-design.md`
for the original conversation-derived rationale this formalizes.
