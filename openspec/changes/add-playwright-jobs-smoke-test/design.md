## Context

See proposal.md - Why. `web/` currently has Vitest only (`web/package.json`'s `"test": "vitest run"`); no browser automation tool is installed anywhere in the repo (checked: no `playwright*` config, no Playwright/Cypress/Puppeteer dependency in `web/`, `design-system/`, or `extension/`). `/jobs` (`web/src/routes/jobs/+page.svelte` + `+page.server.ts`) SSRs a job list anonymously with default filters; `/` is a landing page that only redirects legacy query URLs to `/jobs`.

## Goals / Non-Goals

**Goals:**
- One Playwright test proving the real SSR → Go API → Postgres/Meili pipeline renders job cards in a real browser, running locally against `make up`.
- Minimal, reusable scaffolding (`playwright.config.ts`, one `data-testid`) so a second e2e test is cheap to add later.

**Non-Goals:**
- Not a full e2e suite (no search/filter, auth, or apply-flow coverage).
- No CI integration — this is a local-only check for now; wiring it into CI (spinning up docker-compose in GitHub Actions) is a separate, larger decision left for a future change.
- No Playwright-managed `webServer` — the test does not attempt to boot the app/DB/Meili itself.

## Decisions

- **Target `/jobs`, not `/`.** The homepage renders no job cards (see Context); testing it would either need a different assertion (search box presence) or silently test nothing. `/jobs` is where the "most important" flow — vacancies actually rendering — lives.
- **Assume `make up` is already running rather than a Playwright `webServer`.** `webServer` could start `pnpm --dir web run dev`, but that talks to a dev API server that itself needs `DATABASE_URL`/`MEILI_URL` — reproducing `make up`'s docker-compose topology inside Playwright config would duplicate infrastructure this repo already has a single source of truth for (`docker-compose.yml`). Documenting the precondition (`make up` first) is simpler and matches how the repo's other integration-style checks (e.g. `go test -tags=integration`) already assume Docker is up rather than managing it themselves.
- **Add exactly one `data-testid` (`job-card` on `JobRow.svelte`'s outer div) rather than testing against Tailwind classes.** The repo has zero `data-testid` attributes today; utility classes (`group relative rounded-xl border...`) are a styling contract, not a test contract, and would break the test on a purely visual refactor. `href^="/jobs/"]` was considered as a class-free alternative but a dedicated `data-testid` is the more conventional, extensible choice for future tests (e.g. asserting on card content, not just its wrapper).
- **`baseURL` defaults to `http://localhost:8090`, overridable via `PLAYWRIGHT_BASE_URL`.** Matches `docker-compose.yml`'s `WEB_HOST_PORT` default; the env override lets a differently-configured local setup (or a future CI job) point elsewhere without editing the config file.

## Risks / Trade-offs

- **[Risk] A developer runs `pnpm test:e2e` without `make up` running → the test fails with a connection error, not a clear message.** → Mitigation: none automated in this change (out of scope per Non-Goals); the proposal and any follow-up docs state the precondition. A clearer "is the server up?" pre-check is a reasonable follow-up, not required for the first test.
- **[Risk] `/jobs`'s default SSR result could legitimately be empty** (e.g. a test/staging DB with zero open postings) **→ test fails on infrastructure state, not a code bug.** → Mitigation: accepted for now — this is a smoke test against a normally-populated local `make up` environment, not a hermetic fixture-seeded test; a future change could seed deterministic fixture data if this proves flaky.
- **[Risk] Playwright needs a browser binary installed (`playwright install chromium`) that npm/pnpm install alone doesn't provide.** → Mitigation: documented as a one-time manual step in the task list; not automated into `postinstall` since that would slow down every `pnpm install` in the repo (including CI paths that never run e2e).
