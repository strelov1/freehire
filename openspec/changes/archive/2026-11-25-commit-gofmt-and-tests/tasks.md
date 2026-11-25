## 1. Cursor rule

- [x] 1.1 Add `.cursor/rules/commit-checks.mdc` (`alwaysApply: true`): before a commit that includes `*.go`, run `gofmt -w` on those files (leave `gofmt -l .` empty), `go vet ./...`, and `go test ./...`; do not commit if tests fail; skip the Go suite when no Go files are staged; do not require integration-tagged tests at commit time

## 2. Agent docs

- [x] 2.1 Add a **Before committing** note to `AGENTS.md` (and `CLAUDE.md` if it remains a duplicate) with the same commands, pointing at existing push-time `go vet -tags=integration ./...`

## 3. Guardrails

- [x] 3.1 Confirm CONTRIBUTING.md / CI already match `gofmt -l .` and `go test ./...` — no CI YAML change unless they drifted
