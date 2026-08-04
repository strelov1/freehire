## 1. Store: MergeAutopilotEntry

- [x] 1.1 Add `Store.MergeAutopilotEntry(ctx, id, userID, entry)` in
  `internal/cv/autopilot_store.go`: read the owned CV, replace the entry whose requirement
  text matches (case- and whitespace-insensitively) or append a new one, then
  `SetAutopilotReport` with the merged list.
- [x] 1.2 Tests in `internal/cv/autopilot_test.go`: replaces a matching entry without
  duplicating it, appends when no report exists yet, and is owner-scoped (foreign id →
  `ErrNotFound`).

## 2. cv_edit: requirement / requirement_status

- [x] 2.1 Tests in `internal/handler/assistant_cv_tools_test.go` (already written, RED):
  a call with `requirement` + `requirement_status` merges into the report; a call with
  neither leaves the report untouched; the schema's `requirement_status` enum offers only
  `closed_bank` and `closed_candidate`; a `requirement` with no `requirement_status` is
  refused and the report stays untouched.
- [x] 2.2 Add `requirement` (string) and `requirement_status` (enum: `closed_bank`,
  `closed_candidate`) to `cv_edit`'s schema in
  `internal/handler/assistant_cv_tools.go`.
- [x] 2.3 In the handler: after `cvedit.Commit` succeeds, if `requirement` is set, require
  `requirement_status` (refuse with a message naming the two valid values if absent) and
  call `MergeAutopilotEntry` with an `AutopilotEntry{Requirement, Status, Note: <the edit's
  own note>}`.
- [x] 2.4 Run the task 2.1 tests green; confirm no other `assistant_cv_tools_test.go` case
  regresses. Review found 2 of the delta spec's 4 scenarios untested at the handler level
  (replace-in-place on an existing entry; an explicit invalid non-empty status bypassing
  the schema) — added `TestCVEditToolReplacesAnExistingOpenReportEntry` and
  `TestCVEditToolRejectsAnExplicitOpenRequirementStatus`, both pass immediately (coverage
  gap, not a bug).

## 3. Prompt: tell the agent to use it

- [x] 3.1 Update `tailorPrompt`'s mechanics section in `internal/assistant/prompt.go`: an
  edit that closes a requirement SHOULD pass `requirement` and `requirement_status` on that
  same `cv_edit` call, instead of depending on a separate `tailor_report` call to keep that
  one entry current.
- [x] 3.2 Add a prompt-content test in `internal/assistant/preset_test.go` (matching the
  existing substring-assertion pattern for `tailorPrompt`) pinning that the mechanics
  section mentions `requirement_status`.

## 4. Verify

- [x] 4.1 `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./...`
  — all clean. Also ran `go test -tags=integration ./...` (whole module, Docker): all green,
  no FAIL.
- [x] 4.2 `openspec validate --all --strict`.
