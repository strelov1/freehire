## 1. Right context panel — lift tab, mobile visibility

- [x] 1.1 In `web/src/lib/tailor/ArtifactPanel.svelte`, lift `tab` to a `$bindable('templates')` prop and add a `mobileVisible = false` prop.
- [x] 1.2 Drive the `aside` visibility from `mobileVisible` (conditional `flex`/`hidden`) with a static `lg:flex`; move its fixed pixel width from inline `width:` to a `--w` CSS variable (`w-full lg:w-[var(--w)]`) so it fills the screen on mobile.
- [x] 1.3 Make the panel's own tab bar desktop-only (`hidden … lg:flex`).

## 2. Page — mobile tab bar + region visibility

- [x] 2.1 In `web/src/routes/tailor/[slug]/+page.svelte`, add `mobileView` state and an `artifactTab` state (bound into `ArtifactPanel`), plus a `pickMobile(v)` that sets `mobileView` and syncs `leftTab` / `artifactTab`.
- [x] 2.2 Make the inner container `flex-col lg:flex-row` and add a `lg:hidden` flat, horizontally-scrollable tab bar with the six tabs (Chat, Editor, Preview, Templates, Job, Verdict).
- [x] 2.3 Gate the left panel and centre preview visibility on `mobileView` (conditional `flex`/`hidden` + static `lg:flex`); pass `mobileVisible` and `bind:tab` to `ArtifactPanel`.
- [x] 2.4 Make the left panel's own header (Editor/Chat tabs + save status) desktop-only (`hidden … lg:flex`).

## 3. Verify

- [x] 3.1 `npm run check` (svelte-check) — 0 errors; `npx eslint` on both changed files — clean.
- [ ] 3.2 Visual-verify the mobile layout (throwaway route + headless Chrome per repo convention): confirm each tab shows its view full-width below `lg` and the desktop three-column layout is unchanged at `lg`.
