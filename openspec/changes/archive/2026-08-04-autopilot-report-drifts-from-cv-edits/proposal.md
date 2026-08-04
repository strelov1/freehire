## Why

`tailor_report` and `cv_edit` write two different columns (`cvs.autopilot_report` and
`cvs.document`) through two different tool calls, with nothing tying them together —
`SetCVAutopilotReport` does not even stamp `updated_at`. A model that edits the document
without a follow-up `tailor_report` call leaves a requirement's report entry stale: observed
directly on a real tailored CV, where a PostgreSQL bullet was written into the document but
the report still read the requirement as `open`. The report is what the tailoring workspace
shows beside the fit analysis, so a candidate reloading the page sees a closed requirement
still marked open.

## What Changes

- `cv_edit` gains two optional fields, `requirement` and `requirement_status`. When both are
  set, the edit's own call also merges one entry into the CV's autopilot report — replacing
  the entry for that requirement text if the report already has one, appending it otherwise —
  instead of depending on a second `tailor_report` call the model may not make once the
  requirement is no longer the one it is reasoning about. `requirement_status` accepts only
  the two outcomes an edit can actually produce, `closed_bank` and `closed_candidate` — not
  `open` or `not_reached`, which describe a requirement `cv_edit` never touches.
- A new `Store.MergeAutopilotEntry` (`internal/cv`) does the read-modify-write against the
  owned CV's report.
- `tailorPrompt`'s mechanics section is updated to tell the agent to pass `requirement` /
  `requirement_status` on the edit that closes a requirement, rather than relying on a
  separate `tailor_report` call for it.

## Capabilities

### Modified Capabilities
- `tailor-autopilot`: "A run records a per-requirement report on the tailored CV" gains a
  second way to update one entry — through `cv_edit` in the same call as the edit that closes
  it, not only through a whole-report `tailor_report` call.

## Impact

- Backend: `internal/cv/autopilot_store.go` (new `MergeAutopilotEntry`),
  `internal/handler/assistant_cv_tools.go` (`cv_edit`'s schema and handler),
  `internal/assistant/prompt.go` (`tailorPrompt` mechanics section).
- No migration: `cvs.autopilot_report` already exists (migration 0052).
- No frontend change: the workspace already reads `autopilot_report` from `cv_get`'s response;
  this only makes what it reads fresher.
