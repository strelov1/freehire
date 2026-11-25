## 1. applySeedContent keep-if-empty

- [x] 1.1 In `applySeedContent`, when seeded summary is empty keep the prior document summary; when seeded skills are empty keep the prior skills; non-empty seed still replaces
- [x] 1.2 Unit tests: empty seed preserves keep summary/skills; non-empty seed overwrites; header/margins/style behaviour unchanged

## 2. Reset integration coverage

- [x] 2.1 Update provisional+bank reset test: tailored CV starts with summary/skills; after reset they remain; experience still comes from the bank; seed composition still has no superseded summary
- [x] 2.2 Integration test: current stamp + extract with summary and skills → reset → tailored document carries them (surface-align may rewrite chip wording only on the tailored copy)
- [x] 2.3 Confirm base reseed via the same helper keeps base skills/summary when seed omits them (assertion in an existing or new reset test)

## 3. Guardrails

- [x] 3.1 `go test` for `./internal/handler/` unit tests covering applySeedContent; `go test -tags=integration` for the reset cases; `go vet -tags=integration ./...`
