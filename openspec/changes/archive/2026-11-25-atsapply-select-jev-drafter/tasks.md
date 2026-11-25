## 1. Jev-backed select drafting

- [x] 1.1 Write a failing test for a thin Jev select-choice client (mirrors
      `internal/ai/autofillagent/jev.go`'s shape, package-local to `atsapply`): given a
      question, option labels, and the candidate's stated facts, it builds a `Choice`
      request carrying the categorical-exclusion instruction plus an explicit decline
      alternative, and maps that alternative back to `ok=false`
- [x] 1.2 Implement the client against a mock HTTP server (no real network calls in
      tests). The 6 spike cases became individual unit tests, not a table — a mock
      server can only replay a canned answer, so it cannot exercise real model
      reasoning; the one adversarial case worth asserting against the MOCK is that
      the categorical-exclusion instruction text is actually present in the outgoing
      request for a compensation/legal-status-shaped question
      (`TestJevSelectChooserSendsTheCategoricalExclusionForEveryCall`, added after code
      review flagged its absence). The adversarial cases' real-model behavior was
      verified against the live API in the spike and again in task 3.3's smoke check,
      not re-asserted here against a mock that cannot reason.
- [x] 1.3 Write a failing test for `JevSelectDrafter.Draft` delegating `text`/`textarea`
      fields to the wrapped `LLMDrafter` unchanged, never reaching Jev
- [x] 1.4 Write a failing test for `JevSelectDrafter.Draft` falling back to the wrapped
      `LLMDrafter.Draft` when the Jev call errors on a `select` field
- [x] 1.5 Implement `JevSelectDrafter` (kind dispatch + fallback)
- [x] 1.6 Write a failing test confirming that with `TYPESAFE_API_KEY` unset, no Jev
      request is attempted and drafting behaves exactly like `LLMDrafter` alone for every
      field kind
- [x] 1.7 Implement the unconfigured-passthrough path (`NewJevSelectDrafter` constructor)

## 2. Wiring

- [x] 2.1 Add a `typesafeAPIKey string` field to `atsapply.Client`, threaded through
      `NewClient`'s constructor signature
- [x] 2.2 In `Client.resolve`, wrap `NewLLMDrafter(bound)` with
      `NewJevSelectDrafter(..., c.typesafeAPIKey)` before passing it to
      `ResolveWithDrafting`
- [x] 2.3 Pass `cfg.TypesafeAPIKey` at `atsapply.NewClient(...)`'s one call site in
      `cmd/auto-apply/main.go`
- [x] 2.4 Run the full `internal/api/atsapply` test suite

## 3. Verification

- [x] 3.1 `go build ./...`, `go vet ./...`, `gofmt -l .`
- [x] 3.2 `go test ./...` (all packages pass, no failures)
- [x] 3.3 Manual smoke check against the live Typesafe API with a real key (not committed)
      to confirm the wired path behaves as the spike did, before opening the PR —
      the adversarial compensation case correctly declined against the real API
