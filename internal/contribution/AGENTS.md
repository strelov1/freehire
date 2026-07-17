# Board contribution conventions

## Scope
The crowdsourced "contribute a board" flow: a signed-in user pastes a job link from a
supported multi-tenant ATS, and a company board we do not yet crawl is recorded and rewarded
with a point. Distinct from `internal/submission` (the manual full-card moderation queue) —
contributions are URL-only, auto-validated, unmoderated.

## Always true
- **The unit is the BOARD, not the vacancy.** A contribution is `(source, board)` — the ATS
  provider and the company slug. Two links to the same company (two vacancies, or the bare
  board-listing URL) collapse to one board, so only the first earns a point. Rationale: once
  we know the board, the ingest side onboards it and crawls ALL its vacancies — a second
  vacancy from the same board adds nothing.
- **Board recognition is a pure, network-free URL parse** (`board.go`, `recognizeBoard`): the
  host maps to a source via the `atsBoards` table and the board is the first path segment.
  This is deliberately a small local table, NOT a per-adapter method on `linksource` — for the
  supported ATS the rule is uniform (first path segment), so a table row per ATS is the whole
  cost of adding one. Currently: greenhouse, lever, ashby, workable (all path-based).
  Subdomain-based ATS (recruitee, teamtailor, bamboohr, …) extract the board differently and
  are added when coverage expands — do NOT force them into the first-path-segment rule.
- **Checks run cheapest-first.** unsupported ATS (`ErrUnsupportedATS`, 422) before any DB read;
  board already crawled (`ErrBoardAlreadyTracked`, 409 — a job exists with `external_id`
  prefixed by `<board>:`, via `starts_with`) before any write; the record+point transaction
  last, where a duplicate board (the `UNIQUE (source, board)` on `link_contributions`) surfaces
  as `ErrBoardAlreadyContributed` (409).
- **Record + point are one transaction** (`QueriesRepository.Record`, the `accounts` repo
  pattern), so a rolled-back insert — including the concurrent-duplicate race — credits no
  point. Verified by the build-tagged integration test.
- **Points live on `users.points`**, surfaced on `/auth/me`.

## Entry points (same `Service.Submit`, two front doors)
- **Website:** `POST /api/v1/me/contributions` (`RequireAuthOrKey`), body `{url}`; 201 with the
  recorded board, 422 unsupported, 409 tracked/contributed. `GET /api/v1/me/contributions`
  lists the caller's own.
- **Telegram:** a linked user pastes a board link into the bot chat; `TelegramWebhook`
  (`handler/telegram.go`, `handleTelegramContribution`) resolves the chat to its user
  (`GetUserIDByTelegramChat`), runs the same `Submit`, and replies with the outcome. A message
  with no link is ignored; a link from an unlinked chat prompts the user to link first.

## Limitations
- Points are awarded before the board is verified to fetch (no network on submit). Onboarding
  the board into `sources` and any point claw-back for an unreachable board are deferred to a
  background ingest worker; the `status` column keeps that option open.
- Coverage is the 4 path-based multi-tenant ATS. Subdomain-based and the long tail are a
  follow-up (one `atsBoards`-style rule + test each).
- Migration `0025_link_contributions.sql` (table + `users.points`) applies via Postgres initdb
  only on first volume init — **apply it manually to an existing prod volume BEFORE deploying**.
