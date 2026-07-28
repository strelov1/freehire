# Board contribution conventions

## Scope
The crowdsourced "contribute a board" flow: a signed-in user pastes a job link from a
supported multi-tenant ATS, and a company board we do not yet crawl is recorded and rewarded
with AI credits. Distinct from `internal/submission` (the manual full-card moderation queue) —
contributions are URL-only, auto-validated, unmoderated.

## Always true
- **The board is the unit of REWARD, not the identity of a row.** Only the first submission of
  a `(source, board)` earns credits; later links to the same company are still recorded, each
  naming its own submitter. `UNIQUE (source, board)` was dropped in migration 0047 — it was a
  reward trap (two Microsoft links resolved to one Eightfold board and the promote transaction
  aborted on the duplicate key, so nobody could contribute an already-named employer). "One
  board, one reward" now lives in `Record`: an advisory lock on the board, then an EXISTS test,
  then the insert, all in one transaction. A bare EXISTS would NOT do — two concurrent
  submissions read the same snapshot, both find nothing, and both get paid.
- **Board recognition lives in `internal/atsboard`**, shared with link resolution and
  boardresolve. What remains in `board.go` is the Greenhouse/Ashby job-id parsing, which is
  service logic (it looks the board up in the catalogue by that id). The recogniser is a pure,
  network-free URL parse: the
  host maps to a source + extraction `mode` via the `atsBoards` table. Two modes: `path` (board
  = first path segment, e.g. `jobs.lever.co/<board>`) and `subdomain` (board = leftmost DNS
  label, e.g. `<board>.recruitee.com` — the canonical URL collapses to the bare host). This is
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

## Entry points (one sequence, four doors)
There is no "contribute a board" endpoint any more. Every surface — the website's contribute
form, the Telegram bot, the browser extension, the CLI — posts to `POST /api/v1/jobs/resolve`,
whose sequence lives in `handler/intake.go`: catalog lookup, then import, then record. A second
door onto the same flow is a second behaviour waiting to drift.

`GET /api/v1/me/contributions` still lists the caller's own, now carrying the `surface` each
row came through (`web` | `telegram` | `extension` | `cli` | `unknown`).

Two orderings inside that sequence are load-bearing, both pinned by tests:
- the catalog lookup runs FIRST, or a posting we carry from an aggregator gets a second row
  under `weblink`;
- the board is inspected BEFORE the import (`Service.Inspect`), because the import writes a
  posting under that very board — asking afterwards reports every freshly imported board as
  already tracked.

`Submit` (inspect + record in one call) remains for callers that do not import first.

## Limitations
- Credits are awarded before the board is verified to fetch. Onboarding the board into
  `sources` is still manual (the `onboard-contributions` skill drains the queue); the `status`
  column keeps a background worker open as an option.
- A board may now have SEVERAL rows, so any queue view must group by `(source, board)`.
- Migrations apply via Postgres initdb only on first volume init — **apply
  `0047_link_contributions_surface.sql` manually to an existing prod volume BEFORE deploying**,
  as with `0025` and `0037`.
