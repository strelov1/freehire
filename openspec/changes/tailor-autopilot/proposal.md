## Why

The tailoring workspace opens on a conversation: the agent greets, proposes to walk the gaps, and
waits. Every requirement is then negotiated one message at a time, so the candidate has to drive the
work that the product already knows how to do — their experience bank holds the evidence, the fit
analysis holds the requirement list, and the agent holds the method. Of the six externally-run
tailoring sessions in production, only two ever produced a CV edit; the rest opened the workspace and
left.

A single button that runs the whole method unattended — search the bank for every requirement, rewrite
what the evidence supports, then come back with what is genuinely missing — converts that first minute
from "start a conversation" into "look at a tailored CV". The questions the agent then asks are the
ones only the candidate can answer, and their answers are banked for every future vacancy.

## What Changes

- A tailoring bootstrap no longer auto-sends a kickoff. The empty chat offers two actions:
  **"Tailor it for me"** (the autopilot) and **"Walk me through it"** (today's kickoff text).
  **BREAKING** for the current bootstrap behaviour: the agent no longer starts talking on its own.
- A new server-owned autopilot run: one long agent turn that walks every requirement from the fit
  analysis, searching the experience bank and editing the CV without stopping to ask, and closes by
  recording a run report and asking about the first thing it could not close.
- A new agent tool, `tailor_report`, records the per-requirement outcome of a run
  (`closed_bank` / `closed_candidate` / `open` / `not_reached`) onto the tailored CV.
- The run report renders in the workspace's Verdict panel above the existing fit analysis, with
  "Run again" and "Undo the run" beside it.
- A run snapshots the CV document before its first edit, so the whole run can be reverted in one move.
  Reverting clears the snapshot AND the report.
- The turn's tool-call ceiling becomes a per-turn value chosen by the server (30 for an autopilot run,
  the configured default for every other turn). A client can never ask for a higher ceiling.
- The Editor tab is read-only while a run is in flight, so the page's debounced autosave cannot
  overwrite the agent's edits (or be overwritten by them).

## Capabilities

### New Capabilities
- `tailor-autopilot`: an unattended tailoring run over the vacancy's requirements — its entry point,
  its per-requirement report, the snapshot it takes, and the revert that undoes it.

### Modified Capabilities
- `tailor-workspace`: the bootstrap presents two explicit actions instead of auto-starting the agent,
  and the Editor is read-only for the duration of a run.
- `assistant-agent-runtime`: the maximum tool-call rounds of a turn is chosen per turn by the server
  rather than being a single process-wide constant.

## Impact

- **Schema:** migration `0051` adds `cvs.autopilot_report jsonb` and `cvs.autopilot_undo jsonb`.
- **API:** new `POST /api/v1/assistant/sessions/:id/autopilot` (SSE, cookie-only, tailoring sessions
  only) and `POST /api/v1/me/cvs/:id/autopilot/undo`. The CV read shape gains the report and a
  revertable flag.
- **Go:** `internal/assistant` (`Runner.Run` gains a per-turn config; `tailorPrompt` gains an autopilot
  section), `internal/handler` (the autopilot endpoint, the `tailor_report` tool, the undo handler),
  `internal/db` (queries + regenerated sqlc).
- **Web:** `web/src/routes/tailor/[slug]/+page.svelte` (two-action empty state, run state, editor
  lock, undo ordering) and `web/src/lib/tailor/ArtifactPanel.svelte` (the report block).
- **Not affected:** credits (a tailoring debit is already taken when the tailored CV is created), the
  cached fit analysis (never recomputed from a tailored document), and the experience-bank provenance
  rule (an autopilot edit is subject to the same `evidence_id` wall as a hand-driven one).
