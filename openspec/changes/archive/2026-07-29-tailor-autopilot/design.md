## Context

The tailoring workspace (`/tailor/[slug]`) is a three-column surface: editor/chat on the left, a live
CV preview in the centre, templates/JD/verdict on the right. A tailoring session runs under the
`tailor` preset, whose system prompt (`internal/assistant/prompt.go`) already carries the whole
method: for every requirement, search the experience bank first, rewrite what the evidence supports
citing its `evidence_id`, ask only where the bank is empty, and bank the answer before writing it.

What is missing is not knowledge but rhythm. The agent applies that method one message at a time,
stopping at the first thing it needs to ask, so the candidate drives. This change adds a second
rhythm over the same method: run it all, then come back.

Three facts of the current system shape the design:

1. `Runner.Run` bounds a turn by `r.cfg.MaxSteps` — one number for the whole process, defaulting to
   8. A run over ten requirements needs several times that.
2. The Verdict panel shows the cached fit analysis of the **base** CV against the vacancy
   (`internal/matchanalysis` reads `resumeextract`, never a tailored document), cached one slot per
   (user, job) and stamped with CV-upload time, job `content_hash` and model. A tailored document
   cannot move that number without either a second expensive chain run or clobbering the honest one.
3. The page owns the CV document in memory and autosaves it on an 800 ms debounce, flushing before an
   agent turn and re-reading after. That handshake was written for a turn that makes one or two edits.

## Goals / Non-Goals

**Goals:**
- One click turns an empty tailoring chat into a tailored CV plus an honest list of what is still missing.
- The run is observable while it happens and legible after it: the panel says what was closed and how.
- The run is reversible in one move, so pressing the button is not a gamble.
- The provenance wall (`evidence_id` or no bullet) holds identically in autonomous mode.

**Non-Goals:**
- Recomputing the fit verdict from the tailored document. Expensive, and it would either take a second
  cache slot or overwrite the base-CV analysis the candidate came in with.
- Per-item undo. One snapshot per run; a candidate who wants surgical control uses the editor.
- Metering the run. `credits.FeatureTailor` is already debited when the tailored CV is created, and the
  assistant is unmetered by design; the seam is documented in `internal/assistant/AGENTS.md`.
- A general fix for the editor/agent write race. Locking the editor for the run's duration is scoped to
  where this change makes the race routine.

## Decisions

### One long agent turn, not a server-side loop

The run is an ordinary turn with a raised ceiling, driven by a prompt section, rather than a
deterministic Go loop over requirements (the shape `internal/matchanalysis` uses).

A Go loop would be predictable and easy to test, but it would put a second copy of the tailoring rules
outside the agent — "never invent", "cite the evidence", "bank their words first" — and those rules
would drift from the ones in `tailorPrompt`. Worse, its work would happen outside the transcript, so
the agent the candidate then talks to would not know what it had just done. Keeping the run inside a
turn reuses the transcript, the SSE event stream, the tool registry and the cancellation path, and
costs one prompt section plus one new tool.

The known weakness is that the model may stop early. It is mitigated by the SERVER, not by the
prompt: on reaching the ceiling `Runner.Run` makes its final call with no tools offered, so a
run that spends its whole budget is precisely the one that cannot call `tailor_report`. The
endpoint therefore lays the requirement list down as `not_reached` before the turn starts, and
the agent's own report replaces it. Without that, the worst-case run — edited the CV, stopped
halfway — would be the one whose panel shows nothing and offers no way back.

### A dedicated endpoint owns the brief and the ceiling

`POST /assistant/sessions/:id/autopilot` rather than a `mode` field on the message endpoint.

The ceiling is a spend control on a metered gateway. If it travelled in the request body, any client
could ask for thirty rounds on any session. Server ownership also means the brief text is ours: the
client presses a button, it does not compose an instruction. The endpoint takes the pre-run snapshot
itself, so the snapshot cannot be skipped by a client that forgets to ask for it.

`Runner.Run` grows a per-turn config (`TurnConfig{MaxSteps int}`) where zero means "use the runner's
default", so every existing call site keeps its behaviour.

### The panel shows a run log, not a recomputed verdict

The report is the agent's account of its own run — requirement, outcome, one-line note — persisted on
the CV. This is honest about what it is: a log, not an independent re-score. It costs no LLM call, does
not touch the cached analysis, and answers the question the candidate actually has after a run ("what
did it do, and what is left?").

Alternatives rejected: recomputing the three-stage chain against the tailored document (a second
expensive run, a cache-slot conflict, and a minute of waiting on top of the run itself), and rendering
the log only in the chat (it scrolls away in three messages).

### The report is stored on the CV, replaced whole

Two `jsonb` columns on `cvs` (migration `0052`): `autopilot_report` and `autopilot_undo`. The panel
must render after a reload, and it should not have to parse a transcript to do it. The CV read already
happens after every turn (`onTurnComplete` → `getCv`), so the report rides along with no new endpoint.

`tailor_report` replaces the entire report rather than patching entries. One write path instead of two,
and the later case — a candidate confirms an open requirement in conversation, the agent writes the
bullet and re-reports — is the same call with one status changed. The tool returns `{"saved": n}`: a
tool result is persisted in the transcript and replayed into the model's context on every subsequent
turn, so echoing the report back would be paid for repeatedly.

### The undo clears the report with the document

Restoring the pre-run document while keeping the report would leave the panel claiming edits that no
longer exist. The revert is the whole episode, not just its text.

## Risks / Trade-offs

- **The model stops before the last requirement** → The report distinguishes `not_reached` from `open`,
  and "Run again" resumes. The brief names the requirements to walk, and the closing `tailor_report`
  call makes an incomplete pass visible instead of silent.
- **Thirty rounds is a bigger bill per click than any turn today** → The ceiling is server-owned and
  reachable only through this endpoint, on tailoring sessions, one run at a time per click. If the cost
  proves material, the debit seam is already named.
- **Two runs at once on one CV** → The second snapshot captures a half-edited document, so undo
  returns to the middle of the first run. The client disables both entry points while a turn is in
  flight; the server does not serialise runs, because a per-CV lock is machinery this feature has
  not yet earned. Documented in `internal/cv/AGENTS.md` as a known edge.
- **The editor lock is a coarse instrument** → It blocks a tab for a minute or two of run time. The
  alternative — reconciling two writers on one document — is a much larger change than this feature
  earns, and the lock makes the existing race visible rather than introducing it.
- **The report's requirement text is the model's copy of the analysis's** → Matching is by text, not by
  id, so a paraphrased requirement reads as a different row. Accepted: the report is a display artifact,
  not a join key; the brief tells the agent to copy requirement text verbatim from `cv_context`.
- **An empty experience bank produces a run that closes nothing** → That is the honest outcome, and the
  closing message turns it into an interview. The one-question rule keeps it from becoming a form.

## Migration Plan

Migration `0052` adds two nullable `jsonb` columns; existing rows read as "no run yet" and the panel
falls back to the plain Verdict tab. Nothing backfills. Deploy is migration-first, per the repository's
standing rule that a migration lands before the code that reads it. Rollback is the reverse: the
endpoints stop being called, the columns are inert, and no other behaviour depends on them — except the
bootstrap's two-action empty state, which is a web-only revert.
