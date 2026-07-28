## 1. Pure logic (vitest first)

- [x] 1.1 `clampWidth(px, min, max)` splitter helper in `web/src/lib/tailor/` + vitest (below min → min, above max → max, inside → unchanged)
- [x] 1.2 `splitRequirements(analysis)` → `{ missingHave, missingGap }` from `Analysis.requirement_match` (pure) + vitest, incl. the empty/absent cases

## 2. Extract `<AssistantChat>`

- [x] 2.1 Create `web/src/lib/assistant/AssistantChat.svelte` by moving the chat out of `/my/assistant/+page.svelte`: transport (`RoyClient`), session lifecycle, message list, composer + queue, labels. Props: `{ session?, kickoff?, onTurnComplete?, showSessionRail? }`. Apply the roy-web aesthetic here (no card boxes; thin dividers; centered chat content)
- [x] 2.2 Refactor `/my/assistant/+page.svelte` into a thin host that renders `<AssistantChat showSessionRail />` — chat behaviour unchanged
- [x] 2.3 Verify `/my/assistant`: `svelte-check` clean; visual-verify send / queue / switch-session / delete still work (regression gate for the extraction)

## 3. Artifact panel

- [x] 3.1 `web/src/lib/tailor/ArtifactPanel.svelte`: tabs CV (iframe on `cvPdfUrl(id)?v=N`) · Job description (text) · Verdict (score + recommendation + missing_have/gap via `splitRequirements`); a pointer-capture splitter using `clampWidth`; a `refresh()` that bumps `N`
- [x] 3.2 Verify panel: `svelte-check` clean; visual-verify tab switching + drag-resize

## 4. `/tailor/[slug]` route + own layout

- [x] 4.1 `web/src/routes/tailor/[slug]/+layout.svelte` — full-width own layout (root chrome only; no /my or /jobs chrome)
- [x] 4.2 `web/src/routes/tailor/[slug]/+page.ts` — load the job (`GET /jobs/:slug`) for the JD; SSR the shell
- [x] 4.3 `web/src/routes/tailor/[slug]/+page.svelte` — bootstrap (`api.tailorCv`) → `createSession(tailoring)` → compose `<AssistantChat session kickoff onTurnComplete={panel.refresh} />` + `<ArtifactPanel cvId jobDescription analysis />`; beta gate; 409/error → actionable message + link back to the fit page
- [x] 4.4 Verify the surface: `svelte-check` clean; visual-verify full-width, tabs, resize, live-CV-on-turn, bootstrap-error message

## 5. Entry point

- [x] 5.1 Re-point the "Tailor my CV" CTA on `/jobs/[slug]/fit` to `goto('/tailor/<slug>')` (drop the inline bootstrap/createSession there — the route owns it now)

## 6. Verify

- [x] 6.1 `npm run check` (svelte-check) + `vitest` green; `npm run build` succeeds
- [ ] 6.2 Drive end-to-end: fit CTA → `/tailor/[slug]` full-width, agent auto-starts, tabs switch (CV/JD/Verdict), splitter resizes, CV refreshes after an edit turn; and `/my/assistant` regression (send/queue/switch/delete)
