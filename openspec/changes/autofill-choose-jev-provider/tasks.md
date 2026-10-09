## 1. Config

- [x] 1.1 Add optional `TYPESAFE_API_KEY` to `internal/platform/config` (empty string when
      unset — no `Require()` gate, since the feature must be fully inert without it)
- [x] 1.2 Document `TYPESAFE_API_KEY` in `.env.example` alongside a one-line note that it
      is unrelated to `LLM_*` and only affects the autofill agent's `Choose` calls

## 2. Dependency

- [x] 2.1 ~~Add `github.com/wawan93/gojev` as a direct dependency~~ — reconsidered during
      implementation: the only Go wrapper is a two-commit personal package from the same
      contributor whose `cmd/enrich` Jev PR (#3068) was closed for a bug, and this
      integration only needs one HTTP endpoint. Added no new dependency; implemented a
      minimal internal client instead (`internal/ai/autofillagent/jev.go`). Confirmed no
      official Go client exists either — `langchain-typesafe` is Python-only and alpha
      (see the `hire-typesafe-jev-langchain-python-only` research note).

## 3. Jev-backed Choose

- [x] 3.1 Write a failing test for a thin Jev choice client: given a question, options,
      and profile, it builds a `Choice` request with an explicit "none of these"
      alternative and maps that alternative back to `""`
- [x] 3.2 Implement the client against a mock HTTP server (no real network calls in tests);
      reuse the 8 cases from the 2026-10-08 spike as table-driven fixtures
- [x] 3.3 Write a failing test for `JevPlanner.Choose` falling back to the wrapped
      `LLMPlanner.Choose` when the Jev call errors (network failure and unparseable
      response, as two separate cases)
- [x] 3.4 Implement the fallback in `JevPlanner.Choose`
- [x] 3.5 Write a failing test confirming `JevPlanner.Plan` delegates to
      `LLMPlanner.Plan` unchanged
- [x] 3.6 Write a failing test confirming that with `TYPESAFE_API_KEY` unset, no Jev
      request is attempted and `Choose` behaves exactly like `LLMPlanner.Choose`
- [x] 3.7 Implement the unconfigured-passthrough path

## 4. Wiring

- [x] 4.1 Swap `autofillagent.LLMPlanner{...}` for `autofillagent.JevPlanner{...}` at
      `internal/api/handler/autofill_agent.go:48` (also threaded `TypesafeAPIKey` through
      `handler.Config` and `cmd/server/main.go`, since it did not exist before this change)
- [x] 4.2 Run the full `internal/ai/autofillagent` and `internal/api/handler` test suites

## 5. Verification

- [x] 5.1 `go build ./...`, `go vet ./...`, `gofmt -l .`
- [x] 5.2 `go test ./...` (all packages pass, no failures)
- [x] 5.3 Manual smoke check against the live Typesafe API with a real key (not committed)
      to confirm the wired path behaves as the spike did, before opening the PR —
      caught and fixed a real bug: `model` is a required field (422 without it), not
      optional as the rejected gojev dependency implied via a client-side default
