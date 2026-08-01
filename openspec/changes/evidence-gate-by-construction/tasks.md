## 1. The gate becomes a constructor argument

- [x] 1.1 `internal/handler/cv.go`: `newCVHandlers` takes a `cvedit.EvidenceGate` and
  builds the editor with it (`cvedit.NewEditor(cvedit.NewRepository(pool, queries), gate)`);
  delete the "attached later (withExperienceBank) because the bank is wired after this"
  comment, which is the stale justification.
- [x] 1.2 `internal/handler/handler.go`: hoist
  `experience.NewStore(experience.NewQueriesRepository(queries))` to one variable before
  `newExperienceHandlers`, pass it there, and pass `bankGate{bank: bank}` into
  `newCVHandlers`.

## 2. The mutator and the assistant's ownership of the bank go away

- [x] 2.1 `internal/handler/assistant.go`: `newAssistantHandlers` takes the hoisted bank
  instead of constructing its own, and drops the
  `cvH.editor.WithEvidenceGate(bankGate{...})` block.
- [x] 2.2 `internal/cvedit/editor.go`: delete `Editor.WithEvidenceGate`, and correct
  `NewEditor`'s doc — a nil gate means candidate-only editing, and there is no CLI caller
  that builds one.
- [x] 2.3 `internal/handler/assistant_integration_test.go`: the harness re-does the
  production wiring by hand; construct its editor with the gate instead.

## 3. The duplicate fail-open

- [x] 3.1 `internal/handler/assistant_cv_tools.go`: delete `bankGate.Publishable`'s
  `if g.bank == nil { return nil }` — a second, independent fail-open on the same rule,
  on a field that is never nil.

## 4. Record what was found but not fixed

- [x] 4.1 Add the fixture-divergence finding (three harnesses exercise the agent write
  path with a nil gate, and `cv_tailor_integration_test.go:276` documents the divergence
  as intentional) to `docs/reviews/2026-08-01-architecture-review.md`.

## 5. Verification

- [x] 5.1 `go build ./... && go vet ./... && go test ./...` green; the assistant and CV
  integration suites green under `-tags=integration`; `openspec validate
  evidence-gate-by-construction --strict` passes.
