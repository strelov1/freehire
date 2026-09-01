## 1. Measurement and groundwork

- [x] 1.1 Measure the `maxlength` values that captured `cover_letter_text` fields carry in `apply_forms.payload` on production, and set the two length bands from that distribution. **Done, and the answer was that the data does not exist**: no captured field carries `maxlength` (the stored keys are `id`, `type`, `raw_type`, `label`, `required`, `section`). Measured instead what the postings do offer — 209,297 open postings ask for a letter, 172,783 accept it as text, 36,514 only as a file. `design.md` now sets the bands as a stated product decision and records the file-only share as a limitation.
- [x] 1.2 Register `internal/candidate/coverletter` in the block table in `internal/platform/arch/layering/blocks.go` and confirm `go test ./internal/platform/arch/layering/` passes with an empty package present. `.golangci.yml` needs no entry: `block-candidate` matches on the path glob `**/internal/candidate/**`, so a new package under it is covered by construction.

## 2. Storage

- [x] 2.1 Write `migrations/0120_cover_letters.sql`: one row per `(user_id, job_id)` as the primary key, with `body`, `cited_atom_ids`, `language`, `model`, `created_at`, `updated_at`. Cascade on user and job delete. Run `pnpm check:sql`.
- [x] 2.2 Add the upsert and the owner-scoped read to `internal/platform/db/queries/`, run `make sqlc`, and commit the regenerated code in the same commit as the query.

## 3. The chain

- [x] 3.1 Define the wire shape and the sanitize pass in `internal/candidate/coverletter/coverletter.go`: the letter body, the cited atom ids, the language, and the server-owned bounds (body ceiling, body floor, maximum cited atoms). Test that a model body over the ceiling is clipped and that an out-of-vocabulary field is coerced, not persisted.
- [x] 3.2 Add tolerant decoding of the model's JSON. Solved by narrowing rather than by a `flexdecode.go`: each stage unmarshals into a type naming only the keys its own prompt asks for, so junk in a key the prompt never mentioned cannot fail the stage. A `flexdecode` copy would have been a second answer to a problem the narrow type removes.
- [x] 3.3 Implement the publishable-provenance filter over candidate atoms as a pure function, before any chain code exists. Test that `agent_inferred`, absent, and unrecognised provenance are all withheld, and that `manual` / `cv_import` / `stated_in_chat` pass. Done ahead of 3.2, which only pays off once a stage decodes model output. The filter delegates to `experience.Provenance.Publishable` rather than restating the admissible labels.
- [x] 3.4 Implement Stage 1 (select) in `analyzer.go`: takes the filtered atoms and the `TailoringContext` requirement split, returns the chosen atom ids. Test with a fake LLM that a `missing-have` requirement with a matching publishable atom yields that atom, and that a `missing-gap` with no atom yields none.
- [x] 3.5 Implement Stage 2 (draft): takes the selected atoms, the vacancy and the language, returns a body. Test that the candidate context passed to the model is the `resumeextract.Professional` projection and that raw CV text is never in the request.
- [x] 3.6 Implement Stage 3 (audit) and its merge onto Stage 2. **The three named behaviours are the MODEL's, not the code's** — a scripted fake returns whatever it is told, so a test asserting "the unsupported claim was cut" would assert the fixture. What is testable, and now tested, is that the audit turn actually CARRIES the achievements it is told to check against; the first review found it did not, which made the one mandatory cut unenforceable. The behaviours themselves belong to an llmlive test.
- [x] 3.7 Implement the two degradations: an unparseable Stage 3 serves the sanitized Stage 2 draft, and an audited body below the floor serves the Stage 2 draft. Test both.
- [x] 3.8 Implement the language rule: the letter's language comes from `jobs.posting_language`, falling back to English when it is empty. Test the fallback explicitly, and test that the candidate's profile language is never consulted.
- [x] 3.9 Implement the empty-evidence path: a candidate whose publishable atoms are empty gets no chain run, no model call, and a result naming the reason. Test that no LLM call is made.

## 4. The draft store

- [x] 4.1 Implement `store.go` and `repository.go`: owner-scoped read and upsert over the narrow port, with `WHERE user_id = $1` ownership so another user's draft reports missing rather than forbidden. Test the ownership boundary.
- [x] 4.2 Implement the staleness report: a stored draft whose `model` or `language` differs from the live value reads as stale, and stays readable. Test both stamps independently.
- [x] 4.3 Test that a second draft for the same pair replaces the first and that no history row survives. Needs a real database — a fake Repository cannot exercise `ON CONFLICT` — so it lands as an integration test alongside 5.2.

## 5. HTTP endpoints

- [x] 5.1 Add `GET /api/v1/me/cvs/:id/cover-letter` behind `RequireAuth`: serves the stored draft with its staleness or reports none. Integration-test that no model is called on either path.
- [x] 5.2 Add `POST /api/v1/me/cvs/:id/cover-letter`: loads and authorizes the job, calls `fitanalysis.Required`, runs the chain, upserts the draft. Integration-test the happy path and the no-publishable-evidence refusal.
- [x] 5.3 Test that a mid-chain gateway failure leaves an existing stored draft untouched and returns a failure rather than an empty body. Both halves are covered: `TestDraftFailsWhenTheGatewayFails` in the chain, and `TestCoverLetter_UnproducedDraftLeavesTheStoreAndTheAllowanceAlone` end to end.
- [x] 5.4 Tag the chain's model calls with a new `feature:` value so spend is attributable per candidate from the first deploy, and confirm the call goes out on the candidate's own gateway credential.
- [x] 5.5 Generate the TypeScript wire types via `cmd/gen-contracts` and commit them.

## 6. Assistant tool

- [x] 6.1 Add the `cover_letter_draft` tool to the assistant's registry, calling the same chain and the same store as the endpoint. Test that it acts as the session's authenticated owner.
- [x] 6.2 Test that the tool path applies the provenance gate identically to the endpoint path — the same fixture, both entry points, the same withheld atoms.

## 7. Workspace surface

- [x] 7.1 Add the Cover letter tab to the right panel of `web/src/routes/tailor/[slug]/`, rendering the stored draft, the empty state, and the in-flight state. No score or delta is rendered in this tab.
- [x] 7.2 Render the cited banked achievements alongside the letter, and a copy action for the body.
- [x] 7.3 Disable the draft action while a draft is in flight for that vacancy.
- [x] 7.4 Show a stale draft with its staleness indicated, still readable and still copyable.
- [x] 7.5 Confirm the tab collapses correctly into the existing mobile single-tab view.

## 8. Documentation

- [x] 8.1 Write `internal/candidate/coverletter/AGENTS.md`: the scope, the three stages, the provenance gate and why it is in the service, the language inversion against `matchanalysis`, and the audit floor.
- [x] 8.2 Add the package to the module-files table in the root `CLAUDE.md`, and run `pnpm check:links` so the new relative link resolves.

## 9. Metering (unblocked — add-plan-limits landed on main in #2271)

- [x] 9.1 Add a cover-letter value to `internal/ai/plan`'s `Feature` vocabulary and `AllFeatures`, give it a free-daily and pro-fair-use figure, and reserve against it on the POST path only. The GET path stays free, since it calls no model.
- [x] 9.2 Test that an exhausted allowance refuses the POST with a 402 naming the feature and the reset instant, and that the same account can still read its stored draft. Covered by `TestCoverLetter_ExhaustedAllowanceRefusesTheWriteButNotTheRead`, which also pins that the refusal PRECEDES the chain (no model call) and that the read still serves the stored letter. Enforcement has to be switched on for the test to see a refusal at all, which is itself worth pinning: a shadow-mode deployment never refuses.
