## 1. Web: lift shared match-display helpers

- [x] 1.1 In `web/src/lib/jobMatch.test.ts`, add failing tests for a new exported `toneText(severity)` (hard → destructive tone, medium → warning tone, soft → muted tone) — the same mapping currently private inside `JobMatch.svelte`.
- [x] 1.2 In `web/src/lib/jobMatch.ts`, export `toneText` and the skill-chip class constants (`haveChipClass`, `adjacentChipClass`, `missingChipClass`) so the tests pass.
- [x] 1.3 Update `web/src/lib/components/JobMatch.svelte` to import `toneText`/the chip classes from `jobMatch.ts` instead of defining them locally; remove the now-dead local definitions. No visual/behavioral change.
- [x] 1.4 Run `cd web && npx vitest run jobMatch.test.ts` and confirm green.

## 2. Web: singleton confirm-tailor dialog

- [x] 2.1 Create `web/src/lib/confirmTailorDialog.svelte.ts`: `askConfirmTailor(slug, jobLabel?): Promise<boolean>` that fetches `api.getJobMatch(slug)` on open and resolves from `settleConfirmTailorDialog(value)`, mirroring `web/src/lib/cvRefreshDialog.svelte.ts`'s state/resolver/settle shape (module-level `$state`, no dedicated unit test — this file has no untested precedent in this codebase, consistent with `cvRefreshDialog.svelte.ts`).
- [x] 2.2 Create `web/src/lib/components/ConfirmTailorDialog.svelte`: wraps `ConfirmDialog` from `$lib/ui`, rendering the skills-coverage section (via `match.matched`/`match.missing`/`coverage_percent`), the requirements section (via `partitionBlockers` + `toneText` from `jobMatch.ts`), the no-AI footnote, a loading state while the fetch is in flight, and a fallback message when the fetch fails. Confirm label switches between "Tailor my CV" and "Tailor anyway" based on whether any gap exists.
- [x] 2.3 Mount `<ConfirmTailorDialog />` once in `web/src/routes/+layout.svelte`, alongside the existing `<CvRefreshDialog />`.

## 3. Web: wire the two call sites

- [x] 3.1 `web/src/lib/components/JobDrawer.svelte`: `startTailoring()` awaits `askConfirmTailor(item.job.public_slug, "<title> at <company>")` before `goto(...)`; reset the `tailoring` guard on decline.
- [x] 3.2 `web/src/lib/components/MatchSummary.svelte`: replace the primary CTA's direct `href` with an `onclick` handler that awaits `askConfirmTailor(slug)` then `goto(...)` on confirm; keep the existing guest → `openAuthDialog('login')` branch untouched (auth gate still precedes the new confirmation).
- [x] 3.3 Confirm the "View full analysis" links in both `JobDrawer`'s Fit tab (via `JobMatch.svelte`, unchanged) and `MatchSummary.svelte` remain direct navigation — no confirmation gate added there.

## 4. Extension: widen the wire type and confirm dialog

- [x] 4.1 In `extension/lib/freehire.test.ts`, add failing tests for a new exported `partitionBlockers(blockers)` (unmet sorted by ascending `score_cap` first, then met) mirroring the web equivalent's behavior.
- [x] 4.2 In `extension/lib/freehire.ts`, add the `Blocker` interface (`category`, `severity`, `score_cap`, `reason`, `action`, `met`), add `blockers: Blocker[]` to `JobMatch`, and export `partitionBlockers` so the tests pass.
- [x] 4.3 Run `cd extension && npx vitest run lib/freehire.test.ts` and confirm green.
- [x] 4.4 In `extension/entrypoints/sidepanel/MatchCard.svelte`, change the "Tailor my CV" button from a direct `href`/`target="_blank"` link to opening a local `ConfirmDialog` (from `freehire-design-system`), reusing the `match` prop already loaded for the card (no new fetch) for the skills-coverage and requirements sections, plus the no-AI footnote; `onConfirm` opens `tailorUrl` via `window.open(..., '_blank', 'noopener,noreferrer')`.

## 5. Verification

- [x] 5.1 `cd extension && npm run check && npm run build` — svelte-check and build both clean.
- [x] 5.2 `cd web && npm run check` — svelte-check clean (0 errors; 31 pre-existing warnings in untouched files).
- [x] 5.3 `cd web && npx vitest run` — full web unit suite green (967/967).
- [x] 5.4 `cd extension && npx vitest run` — full extension unit suite green (227/227).
- [x] 5.5 Manual smoke. **Extension**: built the real bundle and Playwright-drove a throwaway Vite harness that mounts the actual `MatchCard.svelte` with fake job/match props (no login required — see the extension-live-verification approach) — confirmed for both states: correct title, correct confirm label ("Tailor anyway" vs "Tailor my CV"), missing-skill chips vs. all-clear line, blocker reasons rendered with correct tone, no-AI footnote present, Cancel closes without navigating, Confirm fires a real `window.open` — screenshots matched the approved mockup. **Web**: `vite dev` boots the SvelteKit app cleanly with the new `ConfirmTailorDialog` mounted in the root layout (no compile/import errors); the interactive confirm/cancel/goto flow was not driven end-to-end here since it requires a running backend + authenticated session (no Docker/API available in this environment) — its logic is the same `ConfirmDialog` primitive and the same `partitionBlockers`/`toneText` helpers already verified in the extension harness and by the unit suite, and `bind:open` follows the exact getter/setter pattern already proven in production by `CvRefreshDialog.svelte`.

## 6. Code review fix

- [x] 6.1 Code review (medium effort, `git diff main...HEAD`) found: `MatchCard.svelte`'s new `blockers`/`hasGaps` derivations ran unconditionally on `match.blockers`, and extension's `partitionBlockers` had no null-guard (unlike web's `?? []`) — but `POST /me/match-text` (App.svelte's ad-hoc, non-catalog page path, `getMatchText`) answers straight from `jobmatch.Compute()`, which never carries a `blockers` field. Every ad-hoc/no-vacancy match card (the exact case the immediately-preceding commit `2f5dee5f9` polished) would throw on mount. Fixed test-first: added a failing `partitionBlockers(undefined)` case to `freehire.test.ts` mirroring the real `/me/match-text` wire shape, then made `JobMatch.blockers` optional and `partitionBlockers` null-safe (`?? []`) in `freehire.ts`, matching the web equivalent exactly. Re-verified: full extension check/build/test suite green, and the harness re-driven with the exact ad-hoc shape (`public_slug: ''`, no `blockers` key) rendered cleanly with zero console errors.
