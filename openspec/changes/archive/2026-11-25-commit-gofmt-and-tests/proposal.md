## Why

Agents and humans commit Go that is not `gofmt`-clean or that has not been unit-tested,
and CI then fails on checks CONTRIBUTING already lists. A commit-time rule in the
agent guidance makes `gofmt` + unit tests a gate before the commit, not after the PR.

## What Changes

- Add an always-on Cursor rule: before creating a git commit that includes Go files,
  run `gofmt` on the dirty Go paths (or `gofmt -l .` clean) and `go test ./...`; do not
  commit if either fails.
- Mirror that in `AGENTS.md` (and the duplicate `CLAUDE.md` if it stays in sync) as a
  **Before committing** step, next to the existing push/vet guidance.
- Point at the same suite CONTRIBUTING/CI already use (`gofmt -l .`, `go test ./...`).
  Do **not** require `go test -tags=integration ./...` on every commit (Docker, minutes);
  keep that as the existing push/behaviour-change guard.

## Capabilities

### New Capabilities

(none — contributor/agent workflow only; `skip_specs: true`)

### Modified Capabilities

(none)

## Impact

- `.cursor/rules/commit-checks.mdc` (new, `alwaysApply: true`)
- `AGENTS.md` / `CLAUDE.md` — commit checklist
- No product code, no git hook, no CI YAML change (CI already enforces the same checks)
