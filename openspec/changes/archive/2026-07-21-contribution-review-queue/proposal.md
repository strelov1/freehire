## Why

When a user contributes a job link the backend can't map to a known ATS, we return 422
and only write a log line — the link is lost unless a maintainer greps prod, and the
contributor gets nothing back but "not a supported ATS board". Real, ingestable sources
hide in that rejected feed (e.g. a Paylocity URL whose pattern the recognizer doesn't know
yet). We want to capture every plausible link so it can be reviewed by hand, while keeping
credit honest — only boards we actually crawl earn a credit.

## What Changes

- An unrecognized-but-valid `http(s)` contribution link is now **recorded** in
  `link_contributions` with a new `review` status (source/board unset) instead of being
  rejected with 422. Non-URL garbage still 422; a link already in the review queue is 409.
- **No credit** is awarded for a `review` row. Credit is still granted only for a novel,
  recognized board (`pending`), exactly as today.
- The contribute UI confirms a review submission with a "we'll check this by hand, not
  credited yet" message, and lists review rows with an "under review · not credited" badge.
- Manual top-up stays a human step (no new binary): once a review link gets an adapter and
  board, a maintainer awards the credit and promotes the row to `onboarded`. The exact SQL
  is documented in the `onboard-contributions` skill.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `link-contributions`: the submit contract gains a third outcome — an unrecognized valid
  link is recorded for manual review (status `review`, no credit) rather than rejected;
  credit is awarded only for recognized novel boards.

## Impact

- Schema: `link_contributions` — `source`/`board` become nullable, status CHECK gains
  `review`, new partial unique index on `url` for the review queue. Prod migration applied
  manually (`SET ROLE hire`).
- Backend: `internal/contribution` (Submit, repository, queries), `internal/handler`
  (CreateContribution reward gate).
- Frontend: `ContributeView.svelte`, `types.ts`.
- Docs: `onboard-contributions` skill gains a review-queue drain + manual-award section.
