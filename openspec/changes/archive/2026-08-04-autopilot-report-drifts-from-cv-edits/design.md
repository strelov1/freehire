## Context

`cvs.autopilot_report` (migration 0052) is a JSONB snapshot the model writes wholesale via
the `tailor_report` tool (`internal/handler/assistant_cv_tools.go`). `cv_edit` writes
`cvs.document` through a completely separate path (`internal/cvedit`). Nothing in the schema
or the handlers ties one write to the other — `SetCVAutopilotReport` (`internal/db/cvs.sql.go`)
doesn't even stamp `updated_at`. The only thing keeping the two in step is the model
remembering, mid-turn, to call `tailor_report` again after an edit that closes a requirement —
stated as a "should" in the tool's own description, not enforced anywhere. Observed directly:
a tailored CV whose document already carried a PostgreSQL bullet while its report still read
that requirement `open`.

## Goals / Non-Goals

**Goals:**
- Let one `cv_edit` call keep the report in step with the edit it just made, without a second
  tool call the model has to remember.
- Keep `tailor_report`'s existing whole-report semantics for the unattended-run path, where it
  already works (`internal/assistant/prompt.go`'s "UNATTENDED RUNS" section) — this change adds
  a second way to touch one entry, it does not replace the first.

**Non-Goals:**
- Making `autopilot_report` a value computed live from the document and the evidence bank
  instead of a value the model asserts. Considered and rejected: the report is phrased in
  requirement-language ("closed_bank", "closed_candidate", a one-line note), not derivable from
  a bullet's `evidence_id` without re-running relevance matching between free-text requirements
  and document paths — a much larger change for what is a status-log staleness bug, not a
  correctness bug in the CV itself.
- A hard guarantee that every requirement-closing edit reports itself. `requirement` /
  `requirement_status` are optional on `cv_edit`; an edit that omits them behaves exactly as
  today. This is the same class of fix as the `tailorPrompt` sequencing rule added earlier in
  this area: it removes the most common way to forget, not every way.

## Decisions

- **Match existing entries by requirement text, case- and whitespace-insensitively; append if
  none matches.** The report has no requirement id — its entries are keyed by the text copied
  verbatim from `cv_context` (`internal/cv/autopilot.go`'s `AutopilotEntry.Requirement`). A
  fresh CV with no prior `tailor_report` call has no matching entry for the first requirement
  it closes, and that must produce a report entry, not a silent no-op.
- **`requirement_status` accepts only `closed_bank` and `closed_candidate`.** `cv_edit` only
  ever closes a requirement; `open` and `not_reached` describe requirements it does not touch.
  Advertising the full four-value vocabulary here would let a model send `open` through a call
  that carries no review of the rest of the report, silently reopening something `tailor_report`
  already closed.
- **The merge happens in the same handler call, after `cvedit.Commit` succeeds, via a second
  read-modify-write (`Store.Get` then `Store.SetAutopilotReport`) rather than a joint
  transaction.** The two writes target different columns for different purposes (the document is
  the CV; the report is a display log of a separate resource), and the existing `tailor_report`
  path is already not transactional with the edits it follows. Consistency here is "good enough
  for a status log", not the same bar as `cvedit`'s own revision/undo guarantees.

## Risks / Trade-offs

- [The model still doesn't pass `requirement`/`requirement_status` on some edit that closes a
  requirement] → Mitigation: `tailorPrompt`'s mechanics section is updated to say so explicitly;
  same class of best-effort fix as the existing "work one requirement at a time" rule.
- [Two concurrent `cv_edit` calls on the same CV race the read-modify-write and one merge is
  lost] → Not mitigated here: `cv_edit` calls within one turn are already sequential (the
  runner issues tool calls one at a time), and cross-session concurrent tailoring of the same CV
  is an existing, unaddressed hazard this change does not widen.
