## Why

GitHub issue #2130 reports the tailoring assistant sometimes files a professional job under
`projects[]` instead of `experience[]` when asked to add or update work history. A prompt-only
fix already landed (`tailor-projects-section-guidance`: the `cv_edit` tool description and tailor
system prompt both state the placement rule) and a test pins that wording, but a system prompt is
advisory — a long conversation or an unusual phrasing can still lead the model to misfile an entry,
and `cv.Project` (`name`, `link`, `bullets`) has no `company`/`start`/`end` fields at all, so a job
filed there doesn't just sit under the wrong heading: its tenure is lost outright, because there is
nowhere on a `Project` to put it.

## What Changes

- Add a structural refusal in `internal/candidate/cvedit`, alongside the existing `ErrForbiddenPath`
  (path policy) and `ErrEvidenceRequired` (evidence gate) refusals in the same package: when an
  **agent** batch (`ActorAgent` only — a candidate's own CLI edit is never refused by this) writes a
  `projects[i]` entry whose `name` carries a `<year> - <year-or-present>` date span, the edit is
  refused with a message naming `experience[]` as the place for job roles with a start/end date.
  This mirrors the existing `listcap.go` guard: a sentinel error, a model-facing `Error()`, and a
  whole-batch refusal (not a partial apply) so the agent sees the refusal and redoes the batch
  correctly inside the same turn.
- The check is narrow by design: a date-range-shaped span in a project's `name` is a strong,
  low-false-positive signal (a real portfolio project's name essentially never has one); bullets are
  NOT checked (a project bullet legitimately mentions years — "cut latency in 2021" — without being
  misfiled), and the check never runs for `ActorCandidate` or `ActorSystem` batches, matching how
  `requireEvidence` is already agent-only.
- No change to the existing prompt-level guidance (`tailor-projects-section-guidance`) — this adds a
  second, structural layer under it, not a replacement.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `cv-tailoring`: a new requirement, alongside "What an actor may edit is an explicit path policy"
  and "Any operation that asserts something about the candidate must cite evidence" — an agent
  batch that files a job-shaped entry (one with a date-range name) under `projects[]` is refused.

## Impact

- `internal/candidate/cvedit/projectshape.go` (new): the detection + sentinel error.
- `internal/candidate/cvedit/editor.go`: hook the check into `commit()`, gated on
  `ch.Actor == ActorAgent`, alongside the existing `refuseIfSanitizeDropsContent` call.
  `CommitDocument` is unaffected — it already refuses `ActorAgent` outright, so a candidate's own
  edit or a résumé reseed is never touched by this.
- `internal/api/handler/cv.go`'s `mapCVError`: a case for the new sentinel, matching how
  `ErrForbiddenPath`/`ErrEvidenceRequired` are mapped today (for the CLI-edit REST path, which can
  carry either actor).
- Unaffected: `internal/ai/assistant/prompt.go`, `internal/api/handler/assistant_cv_tools.go`'s
  `cv_edit` description (the existing prompt guidance stays as-is); no wire-shape change to `Op`,
  `cv.Project`, or `cv.Document`.
