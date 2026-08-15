## 1. Web: lift shared match-display helpers

- [ ] 1.1 In `web/src/lib/jobMatch.test.ts`, add failing tests for a new exported `toneText(severity)` (hard → destructive tone, medium → warning tone, soft → muted tone) — the same mapping currently private inside `JobMatch.svelte`.
- [ ] 1.2 In `web/src/lib/jobMatch.ts`, export `toneText` and the skill-chip class constants (`haveChipClass`, `adjacentChipClass`, `missingChipClass`) so the tests pass.
- [ ] 1.3 Update `web/src/lib/components/JobMatch.svelte` to import `toneText`/the chip classes from `jobMatch.ts` instead of defining them locally; remove the now-dead local definitions. No visual/behavioral change.
- [ ] 1.4 Run `cd web && npx vitest run jobMatch.test.ts` and confirm green.

## 2. Web: singleton confirm-tailor dialog

- [ ] 2.1 Create `web/src/lib/confirmTailorDialog.svelte.ts`: `askConfirmTailor(slug, jobLabel?): Promise<boolean>` that fetches `api.getJobMatch(slug)` on open and resolves from `settleConfirmTailorDialog(value)`, mirroring `web/src/lib/cvRefreshDialog.svelte.ts`'s state/resolver/settle shape (module-level `$state`, no dedicated unit test — this file has no untested precedent in this codebase, consistent with `cvRefreshDialog.svelte.ts`).
- [ ] 2.2 Create `web/src/lib/components/ConfirmTailorDialog.svelte`: wraps `ConfirmDialog` from `$lib/ui`, rendering the skills-coverage section (via `match.matched`/`match.missing`/`coverage_percent`), the requirements section (via `partitionBlockers` + `toneText` from `jobMatch.ts`), the no-AI footnote, a loading state while the fetch is in flight, and a fallback message when the fetch fails. Confirm label switches between "Tailor my CV" and "Tailor anyway" based on whether any gap exists.
- [ ] 2.3 Mount `<ConfirmTailorDialog />` once in `web/src/routes/+layout.svelte`, alongside the existing `<CvRefreshDialog />`.

## 3. Web: wire the two call sites

- [ ] 3.1 `web/src/lib/components/JobDrawer.svelte`: `startTailoring()` awaits `askConfirmTailor(item.job.public_slug, "<title> at <company>")` before `goto(...)`; reset the `tailoring` guard on decline.
- [ ] 3.2 `web/src/lib/components/MatchSummary.svelte`: replace the primary CTA's direct `href` with an `onclick` handler that awaits `askConfirmTailor(slug)` then `goto(...)` on confirm; keep the existing guest → `openAuthDialog('login')` branch untouched (auth gate still precedes the new confirmation).
- [ ] 3.3 Confirm the "View full analysis" links in both `JobDrawer`'s Fit tab (via `JobMatch.svelte`, unchanged) and `MatchSummary.svelte` remain direct navigation — no confirmation gate added there.

## 4. Extension: widen the wire type and confirm dialog

- [ ] 4.1 In `extension/lib/freehire.test.ts`, add failing tests for a new exported `partitionBlockers(blockers)` (unmet sorted by ascending `score_cap` first, then met) mirroring the web equivalent's behavior.
- [ ] 4.2 In `extension/lib/freehire.ts`, add the `Blocker` interface (`category`, `severity`, `score_cap`, `reason`, `action`, `met`), add `blockers: Blocker[]` to `JobMatch`, and export `partitionBlockers` so the tests pass.
- [ ] 4.3 Run `cd extension && npx vitest run lib/freehire.test.ts` and confirm green.
- [ ] 4.4 In `extension/entrypoints/sidepanel/MatchCard.svelte`, change the "Tailor my CV" button from a direct `href`/`target="_blank"` link to opening a local `ConfirmDialog` (from `freehire-design-system`), reusing the `match` prop already loaded for the card (no new fetch) for the skills-coverage and requirements sections, plus the no-AI footnote; `onConfirm` opens `tailorUrl` via `window.open(..., '_blank', 'noopener,noreferrer')`.

## 5. Verification

- [ ] 5.1 `cd extension && npm run check && npm run build` — svelte-check and build both clean.
- [ ] 5.2 `cd web && npm run check` — svelte-check clean.
- [ ] 5.3 `cd web && npx vitest run` — full web unit suite green (confirms no regression from the `JobMatch.svelte` helper move).
- [ ] 5.4 `cd extension && npx vitest run` — full extension unit suite green.
- [ ] 5.5 Manual smoke per the design's two surfaces: extension side panel and web (application drawer + job-page sidebar) — gaps-found state shows both sections and "Tailor anyway"; good-fit state shows the all-clear copy and "Tailor my CV"; Cancel closes without navigating; Confirm reaches `/tailor/[slug]` exactly as before (new tab from the extension, same-tab `goto` from web); guest click on the web sidebar still opens the auth dialog before ever reaching this confirmation.
