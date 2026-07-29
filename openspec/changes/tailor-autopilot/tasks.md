## 1. Storage

- [x] 1.1 Add migration `0051_cvs_autopilot.sql`: `cvs.autopilot_report jsonb` and `cvs.autopilot_undo jsonb`, both nullable, with a comment explaining that one is the run log and the other the pre-run document
- [x] 1.2 Add owner-scoped queries in `internal/db/queries`: snapshot the document into `autopilot_undo`, write `autopilot_report`, read both with the CV, restore from the snapshot clearing both columns; run `make sqlc`
- [x] 1.3 Cover the queries with the existing integration-test harness: snapshot writes only for the owner, restore returns the snapshot and leaves both columns null, restore on a CV with no snapshot reports nothing to revert

## 2. Turn ceiling

- [x] 2.1 Add `TurnConfig{MaxSteps int}` to `internal/assistant` and take it in `Runner.Run`, treating zero as the runner's configured default
- [x] 2.2 Test both paths: a turn naming no ceiling loops up to the configured default (unchanged behaviour), a turn naming one loops up to that instead
- [x] 2.3 Update the existing `Runner.Run` call sites to pass the zero value

## 3. The report tool

- [x] 3.1 Add `tailor_report` in `internal/handler/assistant_cv_tools.go`: schema of `{requirement, status, note}` items, status from the fixed vocabulary, registered on the tailoring preset only
- [x] 3.2 Sanitize before persisting — coerce nothing, refuse an out-of-vocabulary status with a message naming the valid ones, bound item count and text lengths
- [x] 3.3 Persist as a whole-report replacement onto the session's bound CV and return `{"saved": n}`, not the report
- [x] 3.4 Test: a valid report round-trips, an invalid status is refused as a tool error with the CV unchanged, a second call replaces rather than appends

## 4. The run endpoint

- [x] 4.1 Add `POST /api/v1/assistant/sessions/:id/autopilot` (cookie-only, SSE) — owner check, tailoring preset with a bound CV required, a foreign session reported as missing
- [x] 4.2 Take the pre-run snapshot before starting the turn, and run with the server-owned brief and a ceiling of 30
- [x] 4.3 Test: a non-tailoring session is refused with no turn started, a foreign session reports not found, a successful run snapshots the document before the first patch

## 5. The revert endpoint

- [ ] 5.1 Add `POST /api/v1/me/cvs/:id/autopilot/undo` (cookie-only): restore the snapshot, clear the snapshot and the report, return the restored CV
- [ ] 5.2 Test: revert restores the pre-run document and clears both columns; revert without a snapshot is refused and the document is unchanged

## 6. The agent's method

- [ ] 6.1 Add the autopilot section to `tailorPrompt`: walk every requirement from `cv_context` in one turn, `experience_search` each, `cv_edit` what the evidence supports, ask nothing mid-run, close with `tailor_report` then a summary and ONE question
- [ ] 6.2 Add the rule that a requirement closed later from the candidate's own words re-reports with the candidate status
- [ ] 6.3 Integration test on a fake model, modelled on `assistant_profile_tool_integration_test.go`: search → edit → report in one turn, the report persisted, and `cv_edit` without `evidence_id` still refused inside a run

## 7. Wire shape

- [ ] 7.1 Extend the CV read shape with the report and a revertable flag; regenerate the TS contracts with `cmd/gen-contracts`
- [ ] 7.2 Add the two client calls in `web/src/lib/assistant/api.ts` (or the CV client, wherever the tailoring calls live): start a run, revert a run

## 8. Workspace — entry point

- [ ] 8.1 Replace the bootstrap's automatic kickoff with the two-action empty state: "Tailor it for me" starts a run, "Walk me through it" sends today's kickoff text
- [ ] 8.2 Test the empty state: nothing is sent on mount, each action sends exactly its own turn, and a resumed session shows neither

## 9. Workspace — report and revert

- [ ] 9.1 Render the report block in `ArtifactPanel` above `MatchAnalysisFull`, one row per requirement with its outcome, plus "Run again" and "Undo the run"
- [ ] 9.2 Test the rendering of every status, the empty state (no run yet), and that the panel reads the report from the CV the page already re-reads after a turn
- [ ] 9.3 Wire the revert: flush the pending autosave, call revert, re-read the CV, replace the in-memory document — and test that ordering, since a pending save would otherwise resurrect the reverted text

## 10. Workspace — the editor lock

- [ ] 10.1 Make the Editor tab read-only while a run is in flight, with a line saying the agent is editing the CV
- [ ] 10.2 Test that the lock lifts when the run ends and the tab then shows the run's document

## 11. Documentation

- [ ] 11.1 Update `internal/assistant/AGENTS.md` (per-turn ceiling, the autopilot section of the tailoring prompt) and `internal/handler/AGENTS.md` (the two new routes)
- [ ] 11.2 Note the run report and the revert in the tailoring workspace's own notes where the three-column surface is described

## 12. Verification

- [ ] 12.1 `go build ./...`, `go vet ./...`, `go test ./...`, and the web unit tests all green
- [ ] 12.2 Drive a real run in the local stack against a vacancy with a populated experience bank: watch the tool stream, confirm the preview changes, the report renders, the revert restores, and the closing message asks exactly one question
