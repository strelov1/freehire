## 1. The wire reads the scope it is sent

- [x] 1.1 Test (RED): `extension/lib/tools/executor.test.ts` — a `fill_simple` call whose
  fills carry `frame` and `form` produces `LabelFill`s carrying both. Fails today: `readFills`
  returns only `{label, value}`.
- [x] 1.2 `readFills` reads `frame` and `form` off each fill, accepting only integers and
  leaving a missing or non-integer one `undefined` — an unscoped fill stays legal, and a
  malformed scope must not silently become frame 0.

## 2. Go names the form it read a field from

- [x] 2.1 Test (RED): `internal/ai/autofillagent/agent_test.go` — a field `read_form` reports
  in frame 1, form 2 produces a fill carrying both. Fails today: `Field` has no `Form`.
- [x] 2.2 Add `Form int \`json:"form"\`` to `autofillagent.Field` and to `autofillagent.Fill`,
  and carry `field.Form` at the `Fill{...}` construction in `agent.go`.

## 3. The contract holds across the language boundary

- [x] 3.1 Test (RED): the boundary test the defect's shape demands — capture the JSON body Go
  emits for a scoped fill as a fixture, feed it to the extension's `readFills`, assert both
  scopes survive. Decide and record where the fixture lives so it cannot drift from the Go
  struct (a Go test writes it; the TS test reads it).
- [x] 3.2 Make it pass, and confirm it fails when either end is reverted — a boundary test
  that passes against the broken code is the thing this change exists to stop shipping.

## 4. An ambiguous fill refuses

- [x] 4.1 Test (RED): `extension/lib/form.test.ts` — a document with two questions labelled
  "Email" and a fill naming no `form` writes to neither and reports `ambiguous`; the same page
  with a fill naming form 1 writes only there and reports `filled`.
- [x] 4.2 Add `ambiguous` to `FillStatus` in `extension/lib/protocol.ts`; `findQuestion`
  distinguishes "one match" from "several" and `fillByLabel` refuses on the latter.

## 5. An outcome names the obstacle

- [x] 5.1 Test (RED): `form.test.ts` — a fill naming form 0 for a label living only in form 1
  reports `wrong_form`; a fill addressing a question whose only control is disabled or hidden
  reports `not_fillable`; a label nothing carries still reports `not_found`.
- [x] 5.2 Add `wrong_form` and `not_fillable` to `FillStatus`; produce them on the miss path
  in `fillByLabel`/`findQuestion` by re-checking for the control that `collectQuestions`
  filtered out — without keeping unfillable controls in the match set.

## 6. Folding the frames keeps the most informative answer

- [x] 6.1 Test (RED): `executor.test.ts` — `mergeFrameOutcomes` returns `ambiguous` over two
  frames' `not_found`, and the existing "one frame answers, the rest do not hold it" case
  still holds.
- [x] 6.2 Extend `STATUS_RANK` to the full order: `not_found` < `not_fillable` < `wrong_form`
  < `no_option` < `ambiguous` < `deferred_combobox` < `filled`.

## 7. Write down what this change does not close

- [x] 7.1 `extension/AGENTS.md` — the addressing rule (a fill names the scope it was read
  from; ambiguity refuses) beside the existing conventions.
- [x] 7.2 `internal/ai/browsertools/AGENTS.md` — the cross-frame bound, why it is documented
  rather than engineered around, and that `fillByLabel` is the seam if it is ever worth
  closing.

## 8. Verify

- [x] 8.1 `cd extension && npm test && npm run check`; `gofmt -l .` prints nothing;
  `go vet ./...`; `go test ./...`; `go vet -tags=integration ./...`.
- [ ] 8.2 Load the built extension unpacked and run one agent autofill against a real ATS
  page carrying a second form — the failure this change exists for is only observable in a
  browser, and no unit test proves the write landed in the right form on a live page.

## 9. Fixes from review

- [x] 9.1 Test (RED): a fill carrying `form: -1` reaches `fillByLabel` still scoped, and
  matches a question standing outside any `<form>`. `-1` is the documented Ashby shape
  (`protocol.ts`'s `FormField.form`, `formIndex`'s own return), and one `readScope` shared
  by two differently-valued indices discards it — leaving Ashby unscoped AND, on a page
  with a second same-labelled form, newly writing nothing.
- [x] 9.2 Split the reader: `frame` admits `>= 0`, `form` admits `>= -1`.
- [x] 9.3 Test (RED): one planned fill for a label carried by BOTH the application form and
  a job-alert signup in one frame must not write to the signup. `splitByKind` emits one
  `Fill` per matching field, so precise addressing turns the old coin-flip into two
  writes — the harm the proposal opens with, made deterministic.
- [x] 9.4 `readForm` reads `uploads` (the wire already sends them); the agent narrows its
  fields to the application form before planning, mirroring the extension's own
  `scopeToApplication`. A page whose application cannot be identified keeps every field,
  as the extension's does.
- [x] 9.5 Correct the three comments the change falsified: `form.ts`'s `fillByLabel` and
  `planLabelFills` doc blocks and `protocol.ts`'s `LabelFill`, all of which still say an
  unscoped fill matches the first question carrying the label.
- [x] 9.6 `internal/ai/browsertools/AGENTS.md` — the fold cannot report a cross-frame
  collision (it keeps one outcome per label and ties keep the first), and there is no
  hand-authored `fill_simple` caller: the assistant exposes `read_current_page` only.
  Both claims are currently false.
- [x] 9.7 Either extend the boundary fixture test to reject a key `readFills` ignores, or
  retract `browsertools/AGENTS.md`'s claim that it proves the other end reads a new field.
  Today only the fixture UPDATE is enforced.
- [x] 9.8 Note in `findQuestion` that a label repeated INSIDE one form still resolves to
  the first control — design.md reads as though it does not.
