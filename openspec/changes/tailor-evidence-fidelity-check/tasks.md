## 1. Fidelity-check tool

- [x] 1.1 Add a tool function in `internal/handler/assistant_cv_tools.go` (alongside
      `cvJobMatchTool`/`cvEditTool`) that takes `evidence_id`, resolves it via the same
      `experienceBankTools`/`bank.GetAtom` read `bankGate.Publishable` already uses, and returns
      `{claim, context, metrics}`. No model call, no write to CV/report/bank. Reuse `cvToolError`-
      style handling for an id that does not resolve (owner isolation: foreign atom reported as
      missing, not forbidden).
- [x] 1.2 Register the tool in the tailoring session's tool list next to `cv_edit`/`job_match`
      (wherever that list is assembled for the tailoring preset).
- [x] 1.3 Unit test: tool returns the atom's `claim`/`context`/`metrics` for a valid id; returns a
      not-found-style error for an unknown or foreign id; makes no state change.

## 2. Prompt instruction

- [ ] 2.1 Add a `tailorPrompt` paragraph (near the existing evidence-citation instruction) telling
      the agent to call the new tool after a batch of edits that cited evidence, compare its own
      wording against what the tool returns, and revise via `cv_edit` if the wording claims more
      scope, seniority, or a bigger metric than the atom supports. State the same soft-cap framing
      already used for the `job_match` self-check (2-3 rounds, its own judgment to stop, no forced
      convergence).

## 3. Integration coverage

- [ ] 3.1 Scripted-model integration test (`//go:build integration`, mirrors existing
      `internal/handler` assistant CV tool tests): a turn writes a bullet citing evidence, calls
      the fidelity-check tool, and the scripted model's next turn revises the bullet via `cv_edit`
      — assert the tool call and the revision both land in the CV's revision history.
- [ ] 3.2 Scripted-model integration test: the fidelity-check tool is called with an id that does
      not resolve — assert the same not-found-style message a bad `evidence_id` gets elsewhere,
      and that no CV state changes.

## 4. Verification

- [ ] 4.1 `go build ./... && go vet ./...`
- [ ] 4.2 `go vet -tags=integration ./...`
- [ ] 4.3 `go test -tags=integration ./internal/handler/...` (scripted model, no live LLM needed)
- [ ] 4.4 `go test ./...`
