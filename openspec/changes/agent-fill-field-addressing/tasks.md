## 1. The wire reads the scope it is sent

- [ ] 1.1 Test (RED): `extension/lib/tools/executor.test.ts` — a `fill_simple` call whose
  fills carry `frame` and `form` produces `LabelFill`s carrying both. Fails today: `readFills`
  returns only `{label, value}`.
- [ ] 1.2 `readFills` reads `frame` and `form` off each fill, accepting only integers and
  leaving a missing or non-integer one `undefined` — an unscoped fill stays legal, and a
  malformed scope must not silently become frame 0.

## 2. Go names the form it read a field from

- [ ] 2.1 Test (RED): `internal/ai/autofillagent/agent_test.go` — a field `read_form` reports
  in frame 1, form 2 produces a fill carrying both. Fails today: `Field` has no `Form`.
- [ ] 2.2 Add `Form int \`json:"form"\`` to `autofillagent.Field` and to `autofillagent.Fill`,
  and carry `field.Form` at the `Fill{...}` construction in `agent.go`.

## 3. The contract holds across the language boundary

- [ ] 3.1 Test (RED): the boundary test the defect's shape demands — capture the JSON body Go
  emits for a scoped fill as a fixture, feed it to the extension's `readFills`, assert both
  scopes survive. Decide and record where the fixture lives so it cannot drift from the Go
  struct (a Go test writes it; the TS test reads it).
- [ ] 3.2 Make it pass, and confirm it fails when either end is reverted — a boundary test
  that passes against the broken code is the thing this change exists to stop shipping.

## 4. An ambiguous fill refuses

- [ ] 4.1 Test (RED): `extension/lib/form.test.ts` — a document with two questions labelled
  "Email" and a fill naming no `form` writes to neither and reports `ambiguous`; the same page
  with a fill naming form 1 writes only there and reports `filled`.
- [ ] 4.2 Add `ambiguous` to `FillStatus` in `extension/lib/protocol.ts`; `findQuestion`
  distinguishes "one match" from "several" and `fillByLabel` refuses on the latter.

## 5. An outcome names the obstacle

- [ ] 5.1 Test (RED): `form.test.ts` — a fill naming form 0 for a label living only in form 1
  reports `wrong_form`; a fill addressing a question whose only control is disabled or hidden
  reports `not_fillable`; a label nothing carries still reports `not_found`.
- [ ] 5.2 Add `wrong_form` and `not_fillable` to `FillStatus`; produce them on the miss path
  in `fillByLabel`/`findQuestion` by re-checking for the control that `collectQuestions`
  filtered out — without keeping unfillable controls in the match set.

## 6. Folding the frames keeps the most informative answer

- [ ] 6.1 Test (RED): `executor.test.ts` — `mergeFrameOutcomes` returns `ambiguous` over two
  frames' `not_found`, and the existing "one frame answers, the rest do not hold it" case
  still holds.
- [ ] 6.2 Extend `STATUS_RANK` to the full order: `not_found` < `not_fillable` < `wrong_form`
  < `no_option` < `ambiguous` < `deferred_combobox` < `filled`.

## 7. Write down what this change does not close

- [ ] 7.1 `extension/AGENTS.md` — the addressing rule (a fill names the scope it was read
  from; ambiguity refuses) beside the existing conventions.
- [ ] 7.2 `internal/ai/browsertools/AGENTS.md` — the cross-frame bound, why it is documented
  rather than engineered around, and that `fillByLabel` is the seam if it is ever worth
  closing.

## 8. Verify

- [ ] 8.1 `cd extension && npm test && npm run check`; `gofmt -l .` prints nothing;
  `go vet ./...`; `go test ./...`; `go vet -tags=integration ./...`.
- [ ] 8.2 Load the built extension unpacked and run one agent autofill against a real ATS
  page carrying a second form — the failure this change exists for is only observable in a
  browser, and no unit test proves the write landed in the right form on a live page.
