# Plan 002: Cancel server work when extension Stop is pressed

## Status
- Priority: P1
- Effort: S
- Risk: LOW
- Depends on: none
- Category: bug
- Planned at: commit `9a9c4aead`, 2026-11-11

## Why this matters
Stop currently reports cancellation locally while model calls and tools keep running. Deliberate cancellation must use the server contract.

## Current state
`extension/lib/assistant/client.ts:84` returns `{ done, cancel: () => controller.abort() }`. The backend `internal/api/handler/assistant.go:825` uses a detached context and lines 912-920 intentionally ignore reader failures. The route at line 235 accepts `POST /api/v1/assistant/sessions/:id/cancel`. The web exemplar `web/src/lib/assistant/client.ts:44-49,296-303` sends cancellation separately before aborting.
Extension API requests use absolute HIRE_ORIGIN, Authorization Bearer and credentials omit. Never attach the website cookie. Existing cancellation tests in `extension/lib/assistant/client.test.ts` only assert local events. `extension/AGENTS.md` still describes the old backend behavior and is in scope to correct.

## Scope
Only `extension/lib/assistant/client.ts`, `client.test.ts`, `extension/AGENTS.md`. Do not edit App.svelte (another executor owns it), the server, auth policy, or package manifests.
Reviewer maintains index.

## Drift and workflow
Run `git diff 9a9c4aead -- extension/lib/assistant/client.ts extension/lib/assistant/client.test.ts extension/AGENTS.md`; investigate drift. Work in the already isolated task worktree. No commits, pushes, merges.

## Steps
1. Read extension AGENTS and current web cancel exemplar. Extend existing tests: Stop sends POST to encoded session cancel URL with Bearer, credentials omit, no aborted stream signal; cancellation works before response headers and during streaming; rejecting cancel fetch does not leave an unhandled rejection or prevent local termination.
   Verify: `npm --prefix extension test -- lib/assistant/client.test.ts` fails on missing cancel request before implementation.
2. Add a narrow best-effort cancellation request invoked by explicit cancel, then abort the reader. Preserve terminal cancelled behavior and normal streams. Do not change backend disconnect policy; don't silently alter disposal semantics beyond the existing Turn.cancel contract. Correct stale comments in client and extension AGENTS.
   Verify: targeted tests pass, including a cancel request failure.
3. Run `npm --prefix extension test`, `npm --prefix extension run check`, `npm --prefix extension run lint`, `npm --prefix extension run format:check`, `npm --prefix extension run build`: all exit 0. If needed install linked design-system using pnpm then extension with npm ci, without lockfile edits.

## Done criteria
All above checks pass, authenticated cancellation is asserted independently of local abort, no unsupported backend changes, only scope files changed. Use existing Vitest fetch/SSE mocks and `must` helper; avoid timers for new tests where deferred promises suffice.

## STOP conditions
Stop if route cannot authenticate the extension or the change requires new API behavior. Report independent baseline failures; do not rewrite tooling.

## Maintenance
Keep explicit Stop aligned with server cancellation while disconnects remain survivable. Do not transmit credentials in diagnostics.
