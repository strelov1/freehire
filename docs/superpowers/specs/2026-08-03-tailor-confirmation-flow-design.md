# Fix the tailoring assistant's confirmation flow

## Problem

A real production tailoring session (`assistant_sessions` id
`bf58de6a-a83b-4056-aa7e-16e3a908a70a`) got stuck asking the candidate to
"confirm in your own words" 16 times and never recovered — the last message
in the transcript is still the same refusal. Root cause, traced through
`internal/candidate/experience/store.go` and `internal/platform/db/queries/experience.sql`:

`InsertAtomIfNew` dedups on `(user_id, claim_key)` with `ON CONFLICT DO
NOTHING`. Once a claim is first banked with provenance `agent_inferred` (the
model paraphrased it, no verbatim quote backed it), every later
`experience_add` call for that same claim — even one whose `said` field is
now a genuine, verbatim quote from the candidate — hits the conflict, inserts
nothing, and `Store.AddAtom` maps the resulting `pgx.ErrNoRows` straight to
`ErrAlreadyBanked`. The new call's correctly-computed provenance is silently
discarded. There is no other tool that can re-stamp an atom's provenance
after creation (`experience_update`'s schema has no `said`/`provenance`
field). So a claim that fails the verbatim check once is stuck at
`can_write_cv: false` forever, in that session and every session after it.

Separately, the same transcript shows the candidate being asked to
type/paste confirmations by hand, over and over, in plain chat text — a UX
that's needlessly high-friction even once the dead end above is fixed. And
the transcript's `/followups` calls (one after nearly every exchange) are a
feature the user wants removed outright, independent of this bug.

This spec covers three independent, already-agreed fixes. A fourth, larger
idea (borrowing JD-reframing taxonomy and systematic keyword coverage from
`ai-job-search`/`career-ops` to make tailoring rewrite more assertively) is
explicitly out of scope here — separate design pass later.

## A. Provenance upgrade on conflict

**File:** `internal/platform/db/queries/experience.sql` (`InsertExperienceAtomIfNew`),
regenerated into `internal/platform/db/experience.sql.go`; caller
`internal/candidate/experience/store.go:178-194` (`Store.AddAtom`).

Change the insert's conflict clause from `DO NOTHING` to a conditional
upgrade:

```sql
INSERT INTO experience_atoms (user_id, employment_id, claim, claim_key, context, metrics, skills, provenance, source_ref)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (user_id, claim_key) DO UPDATE
SET provenance = EXCLUDED.provenance,
    source_ref = EXCLUDED.source_ref,
    context = EXCLUDED.context
WHERE experience_atoms.provenance = 'agent_inferred'
  AND EXCLUDED.provenance != 'agent_inferred'
RETURNING id, user_id, employment_id, claim, claim_key, context, metrics, skills, provenance, source_ref, created_at, updated_at;
```

Behavior:

- Existing case (nothing to upgrade — already confirmed, or the new call is
  itself still `agent_inferred`): `WHERE` is false, `DO UPDATE` performs no
  write, `RETURNING` yields no row, `pgx.ErrNoRows` fires exactly as today →
  `Store.AddAtom` still returns `ErrAlreadyBanked`. No behavior change for the
  correctly-working case.
- New case (a claim banked as `agent_inferred` gets a later call whose
  `provenanceFor` genuinely computes `stated_in_chat`/`manual`/`cv_import`):
  the row is updated in place, `RETURNING` yields it, `AddAtom` returns the
  now-upgraded atom (`can_write_cv: true`) instead of `ErrAlreadyBanked`. The
  atom's id does not change, so a `cv_edit` call already holding that
  `evidence_id` succeeds on retry with no other change needed.
- `claim`/`skills`/`metrics`/`employment_id` are intentionally NOT
  overwritten on upgrade — an upgrade confirms provenance for the existing
  claim text, it does not let a second call silently rewrite what the claim
  says.

No tool signature changes. `experience_add`'s existing retry-with-`said`
behavior (already what the model does today, per the transcript) starts
working instead of looping forever.

**Testing:** a store-level test that adds a claim with no `said` (lands
`agent_inferred`), then calls `AddAtom` again with a `said` that verbatim-
matches a transcript message (lands `stated_in_chat`), and asserts the
second call returns `can_write_cv: true` with the same atom id. A second test
confirms an already-`stated_in_chat` atom is never downgraded by a later
`agent_inferred` call.

## B. Confirm via button, not free text

**New tool, `tailor` preset only:** `request_confirmation`, added alongside
`assistantCVTools` in `internal/api/handler/assistant_cv_tools.go` (registered
under the same `preset == assistant.PresetTailor` gate in
`assistant_tools.go:359`).

```text
Name: "request_confirmation"
Args: { claim: string, question: string }
Run: no side effect — returns {"status": "awaiting_candidate_response"}
```

`tailorPrompt` (`internal/ai/assistant/prompt.go`) step 2 changes from "ASK them
(...)" free text to: call `request_confirmation` with the exact claim text
and a short question, instead of writing the ask as prose. The model still
composes the claim text itself (unchanged); what changes is how it puts the
question to the candidate.

**Frontend rendering:** `ToolGroupList.svelte` currently renders every tool
call through one generic path (label + collapsed detail line, via
`tool-formatters.ts`). Add a name-conditional branch there for
`request_confirmation`: render the `claim` text plus two buttons, **Да** /
**Нет**.

- **Да** calls the same `submitText(raw: string)` the follow-up chips
  already use (`AssistantChat.svelte:545`) with `raw` = the claim text
  **verbatim, unmodified**. This posts a normal chat message
  (`sendTurn`/`POST .../messages`); because it lands in the transcript as a
  real `RoleUser` message containing the exact claim text, the *next*
  `experience_add` call's `provenanceFor` check (a real substring match
  against transcript messages, unchanged by this spec) genuinely passes —
  no bypass of the honesty gate, just a UI shortcut to producing a message
  that satisfies it.
- **Нет** calls `submitText` with a fixed short message (e.g. "Нет, это не
  так — не добавляй."). The model already knows what to do with a decline
  per the existing prompt ("If they say no, leave it out").

No new SSE event type — this rides the existing `tool_use` event, same as
every other tool call.

**Testing:** a Svelte component test for the new branch in
`ToolGroupList.svelte` (renders buttons for `request_confirmation`, falls
back to generic rendering for every other tool name) and one for each
button's click firing `submitText` with the right payload. Backend: a
`TestPromptOnlyNamesToolsThePresetHas`-style check already guards that the
prompt only names tools the preset registers — no new test needed there
beyond registering the tool correctly.

## C. Remove Follow-ups entirely

Deleted, not flagged off:

- `internal/ai/assistant/followups.go`, `internal/ai/assistant/followups_test.go`
- `internal/api/handler/assistant_followups.go`,
  `assistant_followups_test.go`, `assistant_followups_integration_test.go`
- Route registration in `internal/api/handler/assistant.go:148`
- The route-list assertion in
  `internal/api/handler/assistant_integration_test.go:235`
- The billing tag `tagFollowUps` in `internal/api/handler/user_llm.go:18` (check
  no other reference needs it before deleting)
- Frontend: `web/src/lib/assistant/followups.ts` +
  `followups.test.ts`, the `suggestFollowUps` call in
  `web/src/lib/assistant/api.ts:82-95`, and every reference in
  `AssistantChat.svelte` (state `followUps`, `askForFollowUps`, the reset
  points, the chip-rendering block)

Not touched: `internal/api/handler/followup.go` / `internal/application/followup` — a
same-named but unrelated feature (application follow-up email drafts).

**Testing:** existing test suite should simply have fewer tests after
deletion; no replacement coverage needed for a removed feature.

## Out of scope

Making tailoring rewrite more assertively toward the JD's actual language
(reframing taxonomy, systematic per-keyword pass, possible reviewer/critique
step) — a separate, larger design pass after this one ships.
