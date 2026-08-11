## 1. Pin the selection actions

- [x] 1.1 Change the action bar in `ExperienceBankView.svelte` from `sticky top-0 z-10` to `sticky top-14 z-30`, so it clears the `h-14` `sticky top-0 z-40` `TopBar` instead of pinning behind it
- [ ] 1.2 Verify by hand on a bank long enough to scroll: select two achievements, scroll past the top of the list, and confirm Merge / Tailor / Clear stay fully visible and clickable — this is the reported defect and no automated check covers stacking order

## 2. One kickoff builder for both entries

- [x] 2.1 Extract `profileKickoff(ids: string[]): string` in `web/src/lib/assistant/presets.ts` next to `PROFILE_KICKOFF`: no ids → the stock kickoff, ids → the stock kickoff plus the line naming them. Keep the UUID filter as the single validation rule
- [x] 2.2 Rewrite `entryFromQuery` to parse+validate ids from the query and delegate the text to `profileKickoff`, leaving its observable output byte-identical
- [x] 2.3 Extend `presets.test.ts`: `profileKickoff([])` equals the stock kickoff; `profileKickoff([a,b])` names both; the string `entryFromQuery` produces for `preset=profile&atoms=a,b` equals `profileKickoff([a,b])` (the anti-drift assertion the spec's "both entries ask the same thing" scenario needs)

## 3. The panel component

- [x] 3.1 Add `web/src/lib/components/ExperienceAssistantPanel.svelte` taking `open`, `launch` (`{ id, kickoff }`), `onClose`, and `onBankChanged`; render `AssistantChat` inside `{#key launch.id}` with `preset="profile"`, no `session`, `showSessionRail={false}`, `kickoff={launch.kickoff}`
- [x] 3.2 Docked form at `xl`+: `sticky top-14` column, `h-[calc(100dvh-3.5rem)]`, ~360px, non-modal — no focus trap, no `inert`, no scroll lock, no backdrop. `AssistantChat` needs a bounded-height flex parent (see the `/tailor` host)
- [x] 3.3 Overlay form below `xl`: `fixed inset-0 z-50` with `role="dialog"`, `aria-modal="true"`, a labelled close control, and page-scroll lock while open — shaped after `JobDrawer`
- [x] 3.4 Hide on close (`class:hidden`), never unmount: an unmount cancels a streaming turn. Unmount happens only when `launch.id` changes
- [x] 3.5 Capture the live session id via `onSessionChange` (record it, do NOT navigate) and render a link to `/my/assistant/<id>` so a conversation can be opened full-width
- [x] 3.6 Forward `onTurnComplete` to `onBankChanged`, and `onTurnStateChange` to a `turnActive` the host can read

## 4. Host it in the Experience tab

- [x] 4.1 In `ExperienceBankView.svelte` hold `panelOpen` and `launch`; add `launchInterview(ids: string[])` that increments `launch.id`, sets `launch.kickoff = profileKickoff(ids)`, and opens the panel
- [x] 4.2 Wrap the bank in the two-column flex row with the panel first (left), and make the bank column `min-w-0` so long claims cannot force the panel to shrink
- [x] 4.3 Point both entries at `launchInterview` instead of navigating: the header/empty-state "Add an achievement" button (no ids) and the selection's "Tailor with assistant" action (the selected ids). Both become buttons, not `href`s — drop `tailorHref`
- [x] 4.4 Disable both launch actions while a turn is active, so re-launching cannot abandon a streaming turn mid-answer
- [x] 4.5 Split the reload: `load()` (initial + candidate-initiated, clears the selection) and a refresh used by `onBankChanged` that keeps the selection intersected with the ids still present, so a merge made in chat drops only the id that is gone

## 5. Docs and verification

- [x] 5.1 Record in `web/AGENTS.md` that the Experience tab hosts `AssistantChat` in place (third host, after `/my/assistant` and `/tailor`), and that a new subject requires a remount because `arrival.kickoff` is spent at construction
- [x] 5.2 `cd web && pnpm exec vitest run` and `pnpm exec svelte-check --threshold error`
- [ ] 5.3 Manual pass against the spec's scenarios: bank stays scrollable/selectable/editable beside an open panel at `xl`+; a merge made in conversation updates the list behind it without a reload; the overlay below `xl` covers the bank and its close control returns; closing keeps both the changes and the remaining selection
- [ ] 5.4 Confirm `/my/assistant?preset=profile&atoms=<ids>` still opens the same conversation with the same opening message
- [x] 5.5 `openspec validate experience-assistant-panel --type change --strict`
- [ ] 5.6 Offer a `/blog` changelog entry — the Experience tab gaining an in-place interviewer is user-facing
