## 1. Navigation model

- [x] 1.1 `accountNav.ts`: give every item a `workspace` and a `group`, add the mentor, referrer and vacancies items; add `workspaceFor`, `workspaceNav`, `activeSectionHref` and the workspace list; tests first in `accountNav.test.ts`
- [x] 1.2 Icons and labels for the new items, groups and workspaces (`accountNavIcons.ts`, `i18n/shell.ts`); `shell.test.ts` stays green

## 2. Rendering

- [x] 2.1 Account sidebar (`my/+layout.svelte`) and phone drawer (`AccountNavDrawer.svelte`) render the current workspace's groups; Talent Network invitation only in Job search
- [x] 2.2 Agent/Tailor rail (`AccountNavRail.svelte`) lists Job search + Settings
- [x] 2.3 Mentorship layout drops its tab strip; remove `mentorshipTabs.ts` if orphaned
- [x] 2.4 Referrals: one route per tab (`/my/referrals`, `/offers`, `/incoming`), `?tab=` redirects, landing CTA points at `/offers`
- [x] 2.5 Employer: `/my/employer` (profile) and `/my/employer/vacancies`; the dashboard's tab strip goes

## 3. Switcher

- [x] 3.1 Desktop profile menu becomes the workspace switcher (`HeaderProfileMenu.svelte` + test)
- [x] 3.2 Phone menu's account section lists the same switcher (`HeaderMenu.svelte` + test)

## 4. Verify

- [x] 4.1 `svelte-check`, `eslint`, `vitest` green; screenshot each workspace and both menus against the live API
