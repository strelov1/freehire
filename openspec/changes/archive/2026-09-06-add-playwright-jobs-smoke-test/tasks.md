## 1. Scaffolding

- [x] 1.1 Add `@playwright/test` as a devDependency in `web/` (`pnpm --dir web add -D @playwright/test`).
- [x] 1.2 Install the Chromium browser binary (`pnpm --dir web exec playwright install chromium`).
- [x] 1.3 Add `web/playwright.config.ts`: `testDir: 'e2e'`, `baseURL` = `process.env.PLAYWRIGHT_BASE_URL ?? 'http://localhost:8090'`, no `webServer`.
- [x] 1.4 Add `"test:e2e": "playwright test"` to `web/package.json` scripts.

## 2. Test-first: the smoke test

- [x] 2.1 RED — write `web/e2e/jobs-list.spec.ts`: navigate to `/jobs`, wait for `[data-testid="job-card"]`, assert at least one card visible, assert the first card's anchor `href` matches `/jobs/<slug>`. Run it against `make up` and confirm it fails (no `data-testid` exists yet). Confirmed with real seeded data present (30 jobs copied from a running stack) so the failure is specifically about the missing selector, not an empty catalogue.
- [x] 2.2 GREEN — add `data-testid="job-card"` to the outer card `<div>` in `web/src/lib/components/JobRow.svelte`. Re-run `pnpm --dir web run test:e2e` against `make up` and confirm the test passes.
- [x] 2.3 REFACTOR — checked the test and config read cleanly (clear selector, no leftover debug code); no behavior change needed.

## 3. Verification

- [x] 3.1 Run `pnpm --dir web run test` (Vitest) and confirm the existing unit suite is unaffected. 1505/1505 passed, unchanged from baseline.
- [x] 3.2 Run `pnpm --dir web run lint` and `pnpm --dir web run check` and confirm both stay clean on the new/changed files. `lint` exits 0 (only pre-existing warnings, none in touched files); `check` reports 0 errors (34 pre-existing warnings, none in `JobRow.svelte`).
