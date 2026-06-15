<!--
Do not open a PR unless a maintainer has approved you with `lgtm` on an issue.
PRs from unapproved contributors are auto-closed. Open an issue first.
-->

## What and why

<!-- What does this change do, and why does it matter? Link the approved issue. -->

Closes #

## Checklist

- [ ] I understand this code and can explain how it interacts with the rest of the system.
- [ ] `go build ./...`, `go vet ./...`, and `gofmt -l .` (prints nothing) pass.
- [ ] `go test ./...` passes (and `go test -tags=integration ./...` if I touched DB/handlers).
- [ ] I regenerated committed artifacts when their source changed (`make sqlc` for SQL/migrations, `make gen-contracts` for contract types).
- [ ] For `web/` changes: `npm run check` and `npm run build` pass.
- [ ] This stays within freehire's core/extension boundary (see CONTRIBUTING.md) — it does not bloat the core.
