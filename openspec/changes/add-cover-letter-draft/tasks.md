## 1. Measurement and groundwork

- [x] 1.1 Measure the `maxlength` values that captured `cover_letter_text` fields carry in `apply_forms.payload` on production, and set the two length bands from that distribution. **Done, and the answer was that the data does not exist**: no captured field carries `maxlength` (the stored keys are `id`, `type`, `raw_type`, `label`, `required`, `section`). Measured instead what the postings do offer — 209,297 open postings ask for a letter, 172,783 accept it as text, 36,514 only as a file. `design.md` now sets the bands as a stated product decision and records the file-only share as a limitation.
- [x] 1.2 Register `internal/candidate/coverletter` in the block table in `internal/platform/arch/layering/blocks.go` and confirm `go test ./internal/platform/arch/layering/` passes with an empty package present. `.golangci.yml` needs no entry: `block-candidate` matches on the path glob `**/internal/candidate/**`, so a new package under it is covered by construction.

## 2. Storage

- [x] 2.1 Write `migrations/0120_cover_letters.sql`: one row per `(user_id, job_id)` as the primary key, with `body`, `cited_atom_ids`, `language`, `model`, `created_at`, `updated_at`. Cascade on user and job delete. Run `pnpm check:sql`.
- [x] 2.2 Add the upsert and the owner-scoped read to `internal/platform/db/queries/`, run `make sqlc`, and commit the regenerated code in the same commit as the query.

## 3. The chain

- [x] 3.1 Define the wire shape and the sanitize pass in `internal/candidate/coverletter/coverletter.go`: the letter body, the cited atom ids, the language, and the server-owned bounds (body ceiling, body floor, maximum cited atoms). Test that a model body over the ceiling is clipped and that an out-of-vocabulary field is coerced, not persisted.
- [ ] 3.2 Add `flexdecode.go` for tolerant decoding of the model's JSON, following the pattern in `internal/candidate/atscheck/flexdecode.go`. Test the type-mismatch cases that package already covers.
- [x] 3.3 Implement the publishable-provenance filter over candidate atoms as a pure function, before any chain code exists. Test that `agent_inferred`, absent, and unrecognised provenance are all withheld, and that `manual` / `cv_import` / `stated_in_chat` pass. Done ahead of 3.2, which only pays off once a stage decodes model output. The filter delegates to `experience.Provenance.Publishable` rather than restating the admissible labels.
- [ ] 3.4 Implement Stage 1 (select) in `analyzer.go`: takes the filtered atoms and the `TailoringContext` requirement split, returns the chosen atom ids. Test with a fake LLM that a `missing-have` requirement with a matching publishable atom yields that atom, and that a `missing-gap` with no atom yields none.
- [ ] 3.5 Implement Stage 2 (draft): takes the selected atoms, the vacancy and the language, returns a body. Test that the candidate context passed to the model is the `resumeextract.Professional` projection and that raw CV text is never in the request.
- [ ] 3.6 Implement Stage 3 (audit) and its merge onto Stage 2. Test three behaviours separately: an unsupported experience claim is cut, a statement of interest in the employer survives, and an over-long draft comes back inside the band.
- [ ] 3.7 Implement the two degradations: an unparseable Stage 3 serves the sanitized Stage 2 draft, and an audited body below the floor serves the Stage 2 draft. Test both.
- [ ] 3.8 Implement the language rule: the letter's language comes from `jobs.posting_language`, falling back to English when it is empty. Test the fallback explicitly, and test that the candidate's profile language is never consulted.
- [ ] 3.9 Implement the empty-evidence path: a candidate whose publishable atoms are empty gets no chain run, no model call, and a result naming the reason. Test that no LLM call is made.

## 4. The draft store

- [ ] 4.1 Implement `store.go` and `repository.go`: owner-scoped read and upsert over the narrow port, with `WHERE user_id = $1` ownership so another user's draft reports missing rather than forbidden. Test the ownership boundary.
- [ ] 4.2 Implement the staleness report: a stored draft whose `model` or `language` differs from the live value reads as stale, and stays readable. Test both stamps independently.
- [ ] 4.3 Test that a second draft for the same pair replaces the first and that no history row survives.

## 5. HTTP endpoints

- [ ] 5.1 Add `GET /api/v1/me/cvs/:id/cover-letter` behind `RequireAuth`: serves the stored draft with its staleness or reports none. Integration-test that no model is called on either path.
- [ ] 5.2 Add `POST /api/v1/me/cvs/:id/cover-letter`: loads and authorizes the job, calls `fitanalysis.Required`, runs the chain, upserts the draft. Integration-test the happy path and the no-publishable-evidence refusal.
- [ ] 5.3 Test that a mid-chain gateway failure leaves an existing stored draft untouched and returns a failure rather than an empty body.
- [ ] 5.4 Tag the chain's model calls with a new `feature:` value so spend is attributable per candidate from the first deploy, and confirm the call goes out on the candidate's own gateway credential.
- [ ] 5.5 Generate the TypeScript wire types via `cmd/gen-contracts` and commit them.

## 6. Assistant tool

- [ ] 6.1 Add the `cover_letter_draft` tool to the assistant's registry, calling the same chain and the same store as the endpoint. Test that it acts as the session's authenticated owner.
- [ ] 6.2 Test that the tool path applies the provenance gate identically to the endpoint path — the same fixture, both entry points, the same withheld atoms.

## 7. Workspace surface

- [ ] 7.1 Add the Cover letter tab to the right panel of `web/src/routes/tailor/[slug]/`, rendering the stored draft, the empty state, and the in-flight state. No score or delta is rendered in this tab.
- [ ] 7.2 Render the cited banked achievements alongside the letter, and a copy action for the body.
- [ ] 7.3 Disable the draft action while a draft is in flight for that vacancy.
- [ ] 7.4 Show a stale draft with its staleness indicated, still readable and still copyable.
- [ ] 7.5 Confirm the tab collapses correctly into the existing mobile single-tab view.

## 8. Documentation

- [ ] 8.1 Write `internal/candidate/coverletter/AGENTS.md`: the scope, the three stages, the provenance gate and why it is in the service, the language inversion against `matchanalysis`, and the audit floor.
- [ ] 8.2 Add the package to the module-files table in the root `CLAUDE.md`, and run `pnpm check:links` so the new relative link resolves.

## 9. Metering (sequenced last — depends on add-plan-limits)

- [ ] 9.1 Once `add-plan-limits` has landed its allowance surface, widen the metered-feature vocabulary to admit the cover letter and reserve an allowance on the POST path only. The GET path stays free, since it calls no model.
- [ ] 9.2 Test that an exhausted allowance refuses the POST with a 402 naming the feature and the reset instant, and that the same account can still read its stored draft.
