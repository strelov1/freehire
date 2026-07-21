## 1. Prompt guard (variant A)

- [x] 1.1 RED: add a test in `internal/enrich/langchain_test.go` asserting the
  system prompt instructs whole-currency-unit salary and forbids stripping the
  decimal of a fractional hourly rate (e.g. it contains the `26.08`/`2608`
  counter-example).
- [x] 1.2 GREEN: add the salary guard sentence to the system prompt in
  `internal/enrich/langchain.go` so the test passes.

## 2. Re-enrich corrupted rows

- [x] 2.1 Bump `enrich.Version` from 1 to 2 in `internal/enrich/provider.go` so
  already-enriched jobs re-enqueue through the corrected prompt. (Config
  constant — verified by `go build`/`go vet`, no dedicated unit test.)

## 3. Verify

- [x] 3.1 `go build ./... && go vet ./... && go test ./internal/enrich/...` all
  green.
