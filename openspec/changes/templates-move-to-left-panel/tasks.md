## 1. Move the tab

- [x] 1.1 Add `templates` to the left panel's tab set in `web/src/routes/tailor/[slug]/+page.svelte`, ordered before Settings — a template is picked first, then tuned
- [x] 1.2 Render the template gallery in the left panel, reusing the existing picker unchanged and keeping `onTemplateSelected` wired to the preview
- [x] 1.3 Remove `templates` from `ArtifactPanel.svelte`'s `Tab` union, its tab list and its body
- [x] 1.4 Move the entry in the mobile tab strip so the narrow layout matches the wide one

## 2. Keep the surface coherent

- [x] 2.1 Check the right panel's default tab and its `mobileVisible` condition still make sense with one fewer tab
- [x] 2.2 Check nothing else routes to the templates tab by name (deep links, host props, analytics events)

## 3. Verification

- [x] 3.1 `pnpm test`, `pnpm lint`, `pnpm check` in `web/`
- [x] 3.2 `pnpm run build` — the PR template asks for it on any `web/` change
- [x] 3.3 Design-system gates: `check:tokens` and `check:adoption` (the token count is held per file, so moved markup must not raise it)
- [ ] 3.4 Look at the page: template picks from the left panel, preview re-renders, both panels still resize
