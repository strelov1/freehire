# Board contribution conventions

## Scope
The crowdsourced "contribute a board" flow: a signed-in user pastes a job link from a
supported multi-tenant ATS, and a company board we do not yet crawl is recorded and rewarded
with AI credits. Distinct from `internal/submission` (the manual full-card moderation queue) —
contributions are URL-only, auto-validated, unmoderated.

## Always true
- **The unit is the BOARD, not the vacancy.** A contribution is `(source, board)` — the ATS
  provider and the company slug. Two links to the same company (two vacancies, or the bare
  board-listing URL) collapse to one board, so only the first earns a point. Rationale: once
  we know the board, the ingest side onboards it and crawls ALL its vacancies — a second
  vacancy from the same board adds nothing.
- **Board recognition is a pure, network-free URL parse** (`board.go`, `recognizeBoard`): the
  host maps to a source + extraction `mode` via the `atsBoards` table. `path` (first path
  segment, `jobs.lever.co/<board>`), `pathlocale` (same, skipping a leading `xx-XX` locale —
  Rippling), `pathportal` (the segment before the posting, because SmartRecruiters also serves a
  posting behind a portal segment), `subdomain` (leftmost DNS label, `<board>.recruitee.com`),
  `host` (the whole careers host, whose regional TLD varies — Zoho, Teamtailor), and `hostpath`
  (`<host>/<site>` — Workday). For subdomain/host/hostpath the canonical URL collapses to the
  board itself. Whatever a mode yields, the platform's **own** product hosts (`app.`,
  `dashboard.`, …) are never a tenant — a career site links to them, and `boardresolve` takes
  the first recognized ATS URL it finds in a page. This is
  deliberately a small local table, NOT a per-adapter method on `linksource` — adding an ATS is
  one row + a test. Covers ~37 multi-tenant ATS (greenhouse, lever, ashby, workable, recruitee,
  smartrecruiters, bamboohr, personio, peopleforce, gupy, freshteam, jazzhr, huntflow, deel,
  jobvite, gem, …); hosts were verified against each adapter's public job URL.
- **Fail-safe by design.** A wrong or missing table entry only makes a link *unrecognized*
  (422), never a false board: a bad apex/mode yields an empty board → reject. So adding a
  best-guess host is safe. Excluded on purpose: single-company brands, aggregators, and
  vanity-domain ATS (Workday/Taleo/SuccessFactors/Oracle/Teamtailor-custom/…) whose board lives
  on the client's own domain and cannot be derived from a URL (see the ATS board-recognition
  audit for the full 138-adapter classification).
- **Checks run cheapest-first.** unsupported ATS (`ErrUnsupportedATS`, 422) before any DB read;
  board already crawled (`ErrBoardAlreadyTracked`, 409 — a job exists with `external_id`
  prefixed by `<board>:`, via `starts_with`) before any write; the record+point transaction
  last, where a duplicate board (the `UNIQUE (source, board)` on `link_contributions`) surfaces
  as `ErrBoardAlreadyContributed` (409).
- **`Record` is a single insert** (`QueriesRepository.Record`, the `accounts` repo pattern): it
  persists the contribution row and maps the `UNIQUE (source, board)` violation — including the
  concurrent-duplicate race — to `ErrBoardAlreadyContributed`. Verified by the build-tagged
  integration test.
- **The reward is AI credits, granted separately by the handler** (keyed by the contribution id,
  not inside `Record`). The legacy `users.points` counter was dropped in migration
  `0034_drop_users_points.sql`; the credit balance is the unified per-user reward now.

## Entry points (same `Service.Submit`, two front doors)
- **Website:** `POST /api/v1/me/contributions` (`RequireAuthOrKey`), body `{url}`; 201 with the
  recorded board, 422 unsupported, 409 tracked/contributed. `GET /api/v1/me/contributions`
  lists the caller's own.
- **Telegram:** a linked user pastes a board link into the bot chat; `TelegramWebhook`
  (`handler/telegram.go`, `handleTelegramContribution`) resolves the chat to its user
  (`GetUserIDByTelegramChat`), runs the same `Submit`, and replies with the outcome. A message
  with no link is ignored; a link from an unlinked chat prompts the user to link first.

## Limitations
- Credits are awarded before the board is verified to fetch (no network on submit). Onboarding
  the board into `sources` and any claw-back for an unreachable board are deferred to a
  background ingest worker; the `status` column keeps that option open.
- Coverage is the 4 path-based multi-tenant ATS. Subdomain-based and the long tail are a
  follow-up (one `atsBoards`-style rule + test each).
- Migration `0025_link_contributions.sql` (table + `users.points`) applies via Postgres initdb
  only on first volume init — **apply it manually to an existing prod volume BEFORE deploying**.
