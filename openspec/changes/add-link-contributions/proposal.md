## Why

Our catalogue only grows as fast as the crawl fleet reaches new company boards. Real users
routinely know of companies we do not cover yet — on ATS platforms we already understand — but
have no way to hand them to us. A paste-a-link form that rewards a user for every new *board*
turns that knowledge into coverage: once we learn a board, the ingest side onboards it and
crawls all of its vacancies.

## What Changes

- Add an authenticated **board-contribution** endpoint: a user pastes a job link from a
  supported multi-tenant ATS — a vacancy URL or a bare board-listing URL. We derive the
  company board `(source, board)` from the URL alone (no network fetch), reject a board we
  already crawl or that was already contributed, and otherwise record the board and award the
  user a point. **The unit is the board, not the vacancy** — a second link to the same company
  earns nothing.
- Add a **points balance** to users (`users.points`), incremented once per accepted board.
- Add a **"my contributions"** view: the boards the user discovered, plus their points balance.
- Add a **Telegram front door**: a user who has linked their Telegram chat can paste a board
  link into the bot chat and the webhook runs it through the same contribution flow, replying
  with the outcome.
- Deferred to the ingest worker (out of scope): the board is validated, onboarded into
  `sources`, and its vacancies scraped. This change records and rewards; it does not onboard.

Non-goals (this change): no moderation queue (distinct from the manual `job-submission` flow),
no public leaderboard, no background ingest/onboarding worker, and coverage limited to the
path-based multi-tenant ATS (greenhouse, lever, ashby, workable) — subdomain-based and the
long tail are a follow-up.

## Capabilities

### New Capabilities
- `link-contributions`: authenticated users contribute a company board by pasting a supported
  ATS link (via the website or a linked Telegram chat); the system derives and dedups the board
  without a network fetch, records novel boards, and maintains a per-user points balance.

### Modified Capabilities
<!-- None: board recognition is a small local table in the new capability; the points balance
     is owned by it; the Telegram webhook gains a branch but its linking contract is unchanged. -->

## Impact

- **New code**: `internal/contribution/` (service, repository, board recognizer), a
  `link_contributions` table + `users.points` column (migration via Postgres initdb), sqlc
  queries, handler routes, and a `handleTelegramContribution` branch on the existing webhook
  (plus a `GetUserIDByTelegramChat` reverse lookup).
- **Frontend**: `web/` (SvelteKit SSR) — a contribution page showing the points balance and the
  boards the user discovered.
- **No breaking changes**: additive schema, new routes, an additive webhook branch. `linksource`
  is untouched.
