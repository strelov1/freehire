## Why

`web/` has no browser-level end-to-end test at all — only Vitest unit/component tests, which never boot a real browser against a real server. A single Playwright smoke test that loads `/jobs` and asserts job cards actually render catches the class of failure unit tests structurally cannot: a broken SSR data fetch, a wiring break between the Go API and the SPA, or a regression that leaves the page rendering an empty/error state while every unit test still passes in isolation. This adds only the smallest slice needed to prove that pipeline end-to-end, not a full e2e suite.

## What Changes

- Add `@playwright/test` as a new devDependency in `web/`.
- Add `web/playwright.config.ts`: `testDir: 'e2e'`, `baseURL` defaulting to `http://localhost:8090` (the port `make up`'s docker-compose exposes `web` on), overridable via `PLAYWRIGHT_BASE_URL`. No `webServer` block — the test assumes `make up` (app + postgres + meili) is already running, the same assumption this repo's other integration-style checks make.
- Add `data-testid="job-card"` to the outer card `<div>` in `web/src/lib/components/JobRow.svelte` — the repo has zero `data-testid` attributes anywhere, so this is the minimal addition needed to give the test (and future ones) a stable selector instead of relying on Tailwind utility classes.
- Add `web/e2e/jobs-list.spec.ts`: navigate to `/jobs` (not `/` — the homepage only redirects legacy query URLs to `/jobs` and renders no job cards itself), wait for `[data-testid="job-card"]`, assert at least one card is visible, assert the first card's anchor `href` matches `/jobs/<slug>`. `/jobs` SSRs anonymously with default filters (`LIMIT 20`), so no auth or query params are needed.
- Add a `"test:e2e": "playwright test"` script to `web/package.json`.

Explicitly out of scope: CI wiring (no `.github/workflows` changes), Playwright auto-starting the dev server, and additional scenarios (search/filter, auth) — this change is the first, most-important smoke test plus the minimal scaffolding it needs, not a full suite.

## Capabilities

No product-facing behavior changes — `data-testid` is a non-visual, non-functional attribute, and everything else here is test tooling. `skip_specs: true` is set in this change's `.openspec.yaml`.

## Impact

- **New dependency**: `@playwright/test` in `web/package.json` (devDependency) — requires `pnpm --dir web exec playwright install chromium` (or the relevant browser) once, locally, before the test can run.
- **Modified**: `web/src/lib/components/JobRow.svelte` (one new attribute), `web/package.json` (one new script + devDependency).
- **New files**: `web/playwright.config.ts`, `web/e2e/jobs-list.spec.ts`.
- **No CI, no runtime/production impact.**
