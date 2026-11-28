## Context

See proposal.md — Why. CI already fails PRs on `gofmt -l .` and `go test ./...`
(CONTRIBUTING.md, `.github/workflows/ci.yml`). `AGENTS.md` tells agents to `go test ./...`
and `go vet -tags=integration ./...` before **push**, but the Cursor commit protocol does
not require fmt or tests before `git commit`. There is no `.cursor/rules/` tree yet.

## Goals / Non-Goals

**Goals:**

- Agents must format and unit-test Go before a commit that includes Go files.
- Humans reading AGENTS.md see the same gate.
- Keep the cheap path: no Docker on every commit.

**Non-Goals:**

- Git pre-commit / pre-push hooks (not requested; optional later).
- Running `go test -tags=integration ./...` on every commit.
- Web `pnpm test` / svelte-check as part of this Go-focused rule.
- Changing CI.

## Decisions

### D1 — Cursor always-apply rule, not a hook

Put the gate in `.cursor/rules/commit-checks.mdc` with `alwaysApply: true` so it binds
when the agent is asked to commit. Duplicate a short **Before committing** bullet in
`AGENTS.md` (and `CLAUDE.md` if that file is still a copy) because some agents only
ingest AGENTS.md.

**Why:** The request was “add as a rule”. Hooks fail open if not installed and punish
docs-only commits. Alternatives considered: hook only (misses agents that skip hooks);
AGENTS.md only (weaker in Cursor than an alwaysApply rule).

### D2 — Scope: Go files in the commit

If the staged set contains no `*.go`, skip fmt/test. If it does:

1. `gofmt -w` on those paths (or `gofmt -l .` empty after format).
2. `go test ./...` (unit; no integration tag).
3. Do not commit if tests fail; fix first.

Also run `go vet ./...` when Go changed — already in CONTRIBUTING; cheap and catches
what unit tests miss. Include it in the rule so it is not dropped.

### D3 — Integration stays push-time

Unchanged: `go vet -tags=integration ./...` before every push; full tagged suite when
behaviour (not just a signature) changed. Do not fold that into commit.

### D4 — Docs-only / OpenSpec commits

No Go in the commit → no Go test required. Do not invent a fmt step for markdown.

## Risks / Trade-offs

- **[Risk] `go test ./...` is slow enough that agents skip the rule** → Mitigation: keep
  it unit-only; rule says do not commit on failure, not “skip if slow”.
- **[Risk] Two sources of truth (rule + AGENTS.md) drift** → Mitigation: rule is the
  Cursor-facing copy; AGENTS.md is one short paragraph that points at the same commands.

## Migration Plan

Add the files; no deploy. Rollback: delete the rule and the AGENTS.md paragraph.

## Open Questions

None — hook deferred; integration stays push-time.
