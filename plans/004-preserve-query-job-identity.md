# Plan 004: Distinguish query-identified job pages in extension conversations

## Status
- Priority: P2
- Effort: S
- Risk: MED
- Depends on: 001 (shared panel lifecycle)
- Category: bug
- Planned at: commit `9a9c4aead`, 2026-11-11

## Why this matters
Distinct Greenhouse embedded jobs currently share a conversation key, so the previous job's chat remains active when the form changes.

## Current state
`extension/entrypoints/sidepanel/App.svelte:176-179` returns origin plus pathname, discarding query parameters. `handlePageChange` resets only on changed key. The adjacent sanitizeForLookup explicitly keeps queries, and `internal/ingest/sources/postingurl.go:54-58` reads board from for and posting id from token on Greenhouse embed/job_app pages.
Use a pure helper under extension/lib with Vitest, as existing URL utilities do. Do not log URL secrets. Display redaction and lookup normalization are separate concerns from identity.

## Scope
`extension/entrypoints/sidepanel/App.svelte`, new `extension/lib/assistant/pageIdentity.ts` and `pageIdentity.test.ts`. Do not change backend recognizers, display URL redaction, lookup helper, storage schema, or unrelated ATS behavior. Reviewer owns index.

## Drift and workflow
Run `git diff 9a9c4aead -- extension/entrypoints/sidepanel/App.svelte` and review expected Plan 001 changes before proceeding. Execute after 001 in the same existing isolated task worktree. No commits/pushes/merge.

## Steps
1. Add pure identity tests for distinct Greenhouse token values, distinct boards, equivalent parameter order, tracking-only changes, same ordinary job path with tracking change, different ordinary paths, invalid URLs and fragments/embedded credentials.
   Verify: `npm --prefix extension test -- lib/assistant/pageIdentity.test.ts` demonstrates query identity collapse before fix.
2. Extract and integrate page identity helper preserving only known identity-bearing parameters on recognized Greenhouse embedded form hosts/paths. Canonicalize parameter order. Ordinary URL keys keep previous semantics. No speculative universal ATS parser. Preserve Plan 001's generation protection.
   Verify: targeted tests pass and App.svelte imports and calls the helper in currentPageKey.
3. Run `npm --prefix extension test`, `npm --prefix extension run check`, `npm --prefix extension run lint`, `npm --prefix extension run format:check`, `npm --prefix extension run build`, all exit 0. Install dependencies only if absent without changing lockfiles.

## Done criteria and test plan
All above checks pass; different embedded jobs have different identities; equivalent jobs with tracking-only changes retain identity; the real panel uses the helper. Use existing Vitest describe/it style and literal non-secret fixture URLs.

## STOP conditions
Stop if a generic ATS registry or storage migration is required. Report expected drift from Plan 001; unrelated drift needs review.

## Maintenance
Only add identity parameters when a concrete provider URL requires them. Avoid including auth or tracking data in persistent keys.
