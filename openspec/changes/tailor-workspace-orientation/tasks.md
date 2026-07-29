## 1. An empty conversation always offers the way in

- [x] 1.1 Replace `openingFor(resuming)` with `openingActions()` and have the workspace pass it unconditionally
- [x] 1.2 Update the unit tests to the new contract: the actions are offered regardless of how the workspace was opened, and still send nothing by themselves

## 2. The active pane is legible

- [x] 2.1 Mark the selected tab with the brand tint and heavier weight in the left panel and the artifact panel

## 3. Navigation

- [x] 3.1 Rename the `/my/cvs` section to "Tailor" in the account navigation model, with a test
- [x] 3.2 Duplicate Agent and Tailor beside Inbox in the header menu, reusing the rail's icons

## 4. Verification

- [x] 4.1 `svelte-check` clean, `pnpm run lint` clean, `pnpm test` green, `pnpm run build` green, Go untouched and still green
- [ ] 4.2 Confirm on production: an empty conversation shows both actions, the active tab is obvious, and the header menu carries the three sections
