# Plan 001: Reject stale conversation restoration

## Status
- Priority: P1
- Effort: S
- Risk: LOW
- Depends on: none
- Category: security
- Planned at: commit `9a9c4aead`, 2026-11-11

## Why this matters
An old account's pending transcript read can refill chat after sign-out. The next account can see that old transcript. Navigation and Reset also invalidate pending restoration.

## Current state
`extension/entrypoints/sidepanel/App.svelte:259-274` currently awaits `getSession(remembered.id, token)`, checks only `if (sessionId) return`, then adopts the transcript. `resetChat()` sets `sessionId = null`, which makes that guard ineffective. Sign-out awaits `signOut()` then `newChat()`; sign-in does not clear chat. A pending `recallSession()` also needs invalidation before it resolves.
The Svelte 5 panel owns account/page lifecycle; existing pure helpers under `extension/lib/` and Vitest tests are the preferred pattern. English only, npm in extension, pnpm in linked design-system. Use the CV vocabulary.

## Scope
Only `extension/entrypoints/sidepanel/App.svelte`, new `extension/lib/assistant/conversationRestore.ts` and `conversationRestore.test.ts`, and narrowly necessary panel lifecycle tests under `extension/lib/`. No unrelated match/autofill refactors or API changes.
The index `plans/README.md` is maintained by the reviewer.

## Drift and workflow
Run `git diff 9a9c4aead -- extension/entrypoints/sidepanel/App.svelte`; investigate any changes. Reuse the already isolated BB task worktree; do not create another checkout. Do not commit, push, or merge. A future PR title must use Conventional Commits.

## Steps
1. Read `extension/AGENTS.md`, auth/session storage helpers, and panel restore/reset/signout/navigation/dispatch code. Add deterministic deferred-promise regression tests for old-account restore after sign-out and a subsequent account, old-page response after navigation, reset during storage lookup, and overlapping restores. Test the helper actually used by the panel, not a duplicated model.
   Verify: `npm --prefix extension test -- lib/assistant/conversationRestore.test.ts` must demonstrate the bug before implementation.
2. Implement a small invalidation mechanism capturing generation before the first await. Invalidate at lifecycle transitions (including sign-out initiation, page changes, reset, teardown). Guard success, error, and finally updates; an older restore must not clear a newer restore's loading state. Preserve the guard against a new conversation created while restoration was pending. Consider delayed auth initialization so sign-out cannot be reversed by an old auth result; constrain edits to session lifecycle.
   Verify: the same targeted tests pass; a same-account/same-page valid restore still works.
3. Verify production integration with the helper and all extension checks.
   Verify: `npm --prefix extension test`, `npm --prefix extension run check`, `npm --prefix extension run lint`, `npm --prefix extension run format:check`, `npm --prefix extension run build` exit 0. Install `pnpm --dir design-system install --frozen-lockfile`, then `npm --prefix extension ci` if dependencies absent; do not change lockfiles. Format only changed files.

## Done criteria and test plan
All commands above pass. Deferred regression tests exercise stale success, stale failure, and stale finalization; valid restore is retained. No account A transcript reaches B. Tests live under `lib/**/*.test.ts` because that is the Vitest include pattern. Inspect the final diff for only scoped changes.

## STOP conditions
Report if unrelated files need changes, baseline tooling fails independently, or two reasonable attempts cannot pass a verification. Do not add dependencies or test-only production APIs.

## Maintenance
Future async restore/auth flows must carry the same invalidation identity. Keep the helper small and test its real caller wiring during review.
