## 1. Paths and operations

- [x] 1.1 `internal/cvedit`: `Path` — canonical text form, parser, and validation by reflection over the edited state's json tags (`title`, `template_id`, and the document's own fields)
- [x] 1.2 `Op` (`set` / `insert` / `remove` / `move`) with an optional `evidence_id`, and `State` as the edited whole
- [x] 1.3 Test: parsing round-trips; an unknown field, a malformed index and an out-of-range index are each rejected with a message naming the path
- [x] 1.4 Enumerate the legal paths from the same reflection, for the schema a model reads — 60 addresses against today's 8 named ops

## 2. Apply and its inverse

- [x] 2.1 `Apply(state, ops) (State, []Op, error)` — inverses computed while the old value is in hand, accumulated in reverse order
- [x] 2.2 All-or-nothing: any failing operation leaves the state untouched
- [x] 2.3 Test: apply → inverse → original, for each operation kind and for a mixed batch
- [x] 2.4 Test: a batch whose third operation is invalid applies none of the four

## 3. The differ

- [x] 3.1 `Diff(old, new State) []Op` — scalars compare directly; equal-length lists compare pairwise; differing lengths go through LCS
- [x] 3.2 Collapse an adjacent `remove`+`insert` of similar values into a `set`, so a rewritten bullet does not read as a delete and an add — a collapsed pair of entries recurses into their fields rather than swapping the entry whole
- [x] 3.3 Test: table over scalars, equal-length lists, insertion, deletion, and the collapse — including the threshold either side. Every case first asserts the property that matters: applying the diff to the old state reproduces the new one exactly

## 4. Revisions and the single writer

- [x] 4.1 Migration: `cv_revisions` (actor, origin, batch, title, note, ops, inverse, base version, reverts_id, reverted_at, timestamps) with the `(cv_id, created_at DESC)` index — plus the sqlc queries, including `GetCVForEdit … FOR UPDATE`, which is what serialises two agent turns arriving together
- [x] 4.2 `Editor.Commit` — policy, evidence gate, apply, sanitize, persist state and revision in one transaction
- [ ] 4.2a `Repository`/`Tx` implementation over sqlc + pgx (the editor is unit-tested against a fake; nothing wires it to the pool yet)
- [x] 4.3 `Editor.CommitDocument` — diff against the stored state, then `Commit`; an unchanged save records nothing
- [x] 4.4 Coalescing: amend the newest revision when actor, origin, paths and recency match; replace `ops`, leave `inverse` alone
- [x] 4.5 Trim to the newest 100 revisions in the same statement that inserts
- [ ] 4.6 Make the repository's document write unexported; `Store.Update` and `Store.Patch` cease to be public
- [x] 4.7 Test: coalescing keeps the original inverse; a different path starts a new revision; the feed trims at the cap

## 5. Policy and the evidence gate

- [x] 5.1 Path policy per actor — the agent denied `header.full_name`, `header.email`, `header.phone`, `title`, `template_id`
- [x] 5.2 Evidence gate keyed on paths that assert something about the candidate, now including `experience[*].stack[*]` and `skills[*].items[*]`
- [x] 5.3 Every refusal names what the caller may do instead
- [x] 5.4 Test: the agent cannot write a contact field; the candidate can; an unevidenced stack entry is refused exactly as an unevidenced bullet is; one uncited operation rejects a batch of three

## 6. Undo

- [x] 6.1 `Editor.Revert` — apply the inverse to the current state as a new revision carrying `reverts_id`, stamp `reverted_at` on the original
- [x] 6.2 `Editor.RevertBatch` — undo a run's revisions in reverse order, clearing the run report only for a whole-run revert
- [x] 6.3 An inapplicable inverse fails with 409 naming that the place it changed is gone, leaving the document unchanged
- [x] 6.4 Test: undoing an older revision keeps the newer ones; an undo can itself be undone; two concurrent runs revert independently

## 7. Entry points

- [ ] 7.1 `PUT /me/cvs/:id` → `CommitDocument(candidate, editor)`
- [ ] 7.2 `PUT /me/cvs/:id/template` → `Commit(candidate, template)`
- [ ] 7.3 `PATCH /me/cvs/:id` → path operations (new body shape)
- [ ] 7.4 `cv_edit` tool → `Commit(agent, tailor_agent, batch)`, accepting a batch of operations in one call, with the schema generated from the enumerated paths
- [ ] 7.5 Tailored-copy creation and seeding → a system revision that opens the feed
- [ ] 7.6 `GET /me/cvs/:id/revisions` and `POST /me/cvs/:id/revisions/:rid/undo`, cookie-only
- [ ] 7.7 Retire `POST /me/cvs/:id/autopilot/undo` in favour of the batch revert
- [ ] 7.8 Test: every entry point leaves a revision; the actor is never read from the request body

## 8. Titles

- [ ] 8.1 Generate a revision's description from its operations and the document — "Rewrote a bullet in Senior Engineer, Acme", "Edited 3 bullets in Acme", "Changed typography"
- [ ] 8.2 Carry the agent's `note` separately, rendered as the agent's words
- [ ] 8.3 Test: a table of operation batches to descriptions, including the folding of several operations into one line

## 9. Workspace

- [ ] 9.1 History tab in `ArtifactPanel` — entries newest first, author, time, description, undo control, run grouping
- [ ] 9.2 `CvHtmlPreview`: carry each item's document index through the empty-entry filtering
- [ ] 9.3 Underline the paths of the hovered, pinned, or newest-run revision
- [ ] 9.4 Move the whole-run revert from `AutopilotReport` into the run group in the feed
- [ ] 9.5 Test (`vitest`): path-to-node matching, including an empty bullet before the changed one; description rendering

## 10. Retirement and clients

- [ ] 10.1 Stop reading and writing `cvs.autopilot_undo`
- [ ] 10.2 Second migration, separate release: drop `cvs.autopilot_undo`
- [ ] 10.3 Update `freehire-cli` to the new `PATCH` body
- [ ] 10.4 `internal/cv/AGENTS.md` and `internal/handler/AGENTS.md`: the single writer, the path policy, the evidence paths

## 11. Verification

- [ ] 11.1 `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`
- [ ] 11.2 `make sqlc` after the migration; contracts regenerated for the new wire types
- [ ] 11.3 Web `pnpm check` and `vitest`
- [ ] 11.4 Drive it: edit in the workspace, run the agent, undo one edit and a whole run, confirm the underline lands on the right line
