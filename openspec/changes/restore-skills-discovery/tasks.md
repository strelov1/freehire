# Tasks

## 1. Re-request `skills` in the prompt and schema

- [ ] 1.1 **RED** — In `internal/enrich/langchain_test.go`:
  - Drop `"skills"` from the hardcoded list in
    `TestSystemPromptOmitsDictBackedFacets` (it must keep asserting the other
    eight dict-backed facets are absent).
  - Add `"skills (array of lowercase tokens"` (or the field name `"skills"`) to
    the fields `TestSystemPromptKeepsServedAndHybridFields` asserts are
    present in the built prompt.
  Confirm both fail against the current prompt (skills still absent).
- [ ] 1.2 **RED** — In `internal/enrich/schema_test.go`, add an assertion
  (either inside `TestRequestSchema_CarriesTheFieldsThePromptAsksFor` or a new
  test) that `schemaProps(t, true)` contains `"skills"`. Confirm it fails.
- [ ] 1.3 **GREEN** — Edit `buildSystemPrompt` in
  `internal/enrich/langchain.go`: add `skills (array of lowercase tokens, e.g.
  go, postgresql), ` back into the "Other keys" line, in the same position it
  held before #659 (immediately after the salary fields).
- [ ] 1.4 **GREEN** — Edit `unaskedFields` in `internal/enrich/schema.go`:
  remove `"skills"` from the slice.
- [ ] 1.5 **Verify green** — `go test ./internal/enrich/...`. All four tests
  from 1.1/1.2 pass; no other test in the package regresses.

## 2. Reconcile comments that now describe stale behavior

- [ ] 2.1 In `internal/enrich/schema.go`, reword the `unaskedFields` doc
  comment: it currently lists `skills` among the dict-served/unasked fields —
  drop it from that list and note it is requested again as a discovery facet.
- [ ] 2.2 In `internal/enrich/enrichment.go`, reword the `servedScalarEnums`
  and `Validate` doc comments: both currently describe `skills` as dict-only
  alongside `countries`. Update the wording so `skills` reads as an active
  discovery facet (like `countries`/`regions`), still deliberately unvalidated
  — no change to `Validate`'s actual logic, comment-only.
- [ ] 2.3 `go build ./... && go vet ./...` — confirm no other reference to the
  old wording (e.g. package-level doc comments quoting the same field list)
  was missed. `grep -rn "skills" internal/enrich/*.go` and eyeball every hit.

## 3. Spec sync

- [ ] 3.1 Confirm `openspec/changes/restore-skills-discovery/specs/ai-enrichment/spec.md`
  (already written alongside this proposal) matches the shipped prompt/schema
  wording exactly — field lists in the spec's "Unserved discovery facets are
  captured raw, not validated" requirement must name `skills` alongside
  `countries`/`regions` as requested, and the other eight as still unrequested.
- [ ] 3.2 `openspec change validate restore-skills-discovery --strict` (or the
  project's equivalent openspec validation command) passes.

## 4. Final check

- [ ] 4.1 Full suite: `go test ./...`.
- [ ] 4.2 Confirm no `enrich.Version` bump and no backfill/migration code was
  added anywhere in the diff — this ships forward-only, per the design.
