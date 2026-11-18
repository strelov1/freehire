# freehire verification map

The maintained list of user-facing features and how to prove each one in a running
instance. Read this index, pick the files your change touches, follow their recipes.

## Baseline preconditions

- An instance from `.agents/skills/verify-freehire/stack.sh up` in this checkout, and
  `stack.sh doctor` all `ok` (built from the current `HEAD` and source snapshot).
- Seed: three fixture postings through the real ingest path by default. Use
  `VERIFY_SEED_MODE=live` for the external `greenhouse/vercel` crawl smoke test.
- `eval "$(.agents/skills/verify-freehire/stack.sh env)"` for `PLAYWRIGHT_BASE_URL`,
  `VERIFY_API_URL`, `DATABASE_URL`, `VERIFY_EVIDENCE`.

## Driving conventions

- Browser journeys: `.agents/skills/verify-freehire/run-journey.sh <spec>`. Journeys live
  in `../journeys/`.
- Wait for `networkidle` after a `goto` that is followed by input (hydration).
- Roles with accessible names and `data-testid` before CSS or position.
- Accounts are created fresh per run (`verify+<ms>@example.com`); nothing to restore.

## Proof and skip reporting

- Each journey prints its unique directory under `$VERIFY_EVIDENCE`; screenshots, logs and traces stay together there.
- A mutation is proven by a second read: the database row, or the page that lists it.
- Report what could not be driven (mail, payments, OAuth, LLM features) as not verified.

## Feature entry contract

Each file: an H1, one paragraph, then `Sub-features`, `How to get to it (user POV)`,
`Driving it with Playwright`, `Gotchas`.

## Features

- [Job feed and search](./jobs-search.md): the `/jobs` feed, keyword search, opening a posting.
- [Sign up and save a job](./signup-and-save.md): email + password registration, saving to the tracking board.
- [Public search API](./public-api.md): `/api/v1/jobs/search`, a posting by slug, the dropped-filter report.
- Mobile account navigation: run `verify.sh web/e2e/account-shell.spec.ts` to check
  the drawer on CV readiness and profile pages at a 375px viewport.
