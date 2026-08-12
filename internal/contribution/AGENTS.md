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
- **Board recognition lives in `internal/atsboard`**, shared with link resolution (board
  coverage) and boardresolve — it used to live here, which `boardresolve` already reached
  across for. What remains in `board.go` is the Greenhouse/Ashby job-id parsing, which IS
  service logic (it looks the board up in the catalogue by that id). The recogniser is a pure,
  network-free URL parse: the
  host maps to a source + extraction `mode` via the `atsBoards` table. `path` (first path
  segment, `jobs.lever.co/<board>`), `pathlocale` (same, skipping a leading `xx-XX` locale —
  Rippling), `pathportal` (the segment before the posting, because SmartRecruiters also serves a
  posting behind a portal segment), `subdomain` (leftmost DNS label, `<board>.recruitee.com`),
  `host` (the whole careers host, whose regional TLD varies — Zoho, Teamtailor), and `hostpath`
  (`<host>/<site>` — Workday). For subdomain/host/hostpath the canonical URL collapses to the
  board itself. Whatever a mode yields, the platform's **own** product hosts (`app.`,
  `dashboard.`, …) are never a tenant — a career site links to them, and `boardresolve` takes
  the first recognized ATS URL it finds in a page. The same is true along the path:
  `reservedSegments` lists, per host, the platform words that are never a tenant, skipped in
  path mode (Jobvite's `careers/<board>/jobs` portal segment) and declining the URL when nothing
  else remains (Greenhouse's `embed/job_app`, whose board lives in the `for=` param). A separate
  `apiBoards` table covers each ATS's **own API host**, where the board sits behind a fixed
  prefix (`api.ashbyhq.com/posting-api/job-board/<board>`) — that XHR is often the only place a
  vanity careers page names its board, and matching it first also stops
  `boards-api.greenhouse.io` from being read as the tenant `v1`. This is
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
  prefixed by `<board>:`, matched by a LIKE-prefix the `(source, external_id
  text_pattern_ops)` index serves as a range scan; `starts_with()` would seq-scan the
  whole source) before any write; the record+point transaction
  last, where a duplicate board (the partial unique index on `link_contributions (source, board)
  WHERE status <> 'rejected'`) surfaces as `ErrBoardAlreadyContributed` (409).
- **`Record` is a single insert** (`QueriesRepository.Record`, the `accounts` repo pattern): it
  persists the contribution row and maps the unique violation — including the
  concurrent-duplicate race — to `ErrBoardAlreadyContributed`. Verified by the build-tagged
  integration test.
- **A rejected board releases its identity.** The uniqueness covers the LIVE statuses only
  (`pending`/`review`/`onboarded`), so a board turned down as dead can be contributed again once
  the employer resumes posting — which opens a new pending row. Spanning `rejected` too would
  have made a single bad day permanent (migration 0049).
- **The reward is AI credits, granted separately by the handler** (keyed by the contribution id,
  not inside `Record`). The legacy `users.points` counter was dropped in migration
  `0034_drop_users_points.sql`; the credit balance is the unified per-user reward now.

## Entry points (one sequence, four doors)
There is no "contribute a board" endpoint any more. Every surface — the website's contribute
form, the Telegram bot, the browser extension, the CLI — posts to `POST /api/v1/jobs/resolve`,
whose sequence lives in `handler/intake.go`: catalog lookup, then import, then record. A second
door onto the same flow is a second behaviour waiting to drift. `GET /api/v1/me/contributions`
still lists the caller's own, now carrying the `surface` each row came through
(`web` | `telegram` | `discord` | `extension` | `cli` | `unknown`).

The intake answers with five outcomes, all of them about the BOARD: `found` (already carried),
`tracked` (imported, and we already crawl this board), `imported` (imported, board queued for
onboarding), `review` (imported, but the URL names no board we can crawl — a careers site on the
company's own domain — so the link went to triage), `queued` (unreadable page, link filed for
triage).

**`found` is reached two ways, and only one of them returns early.** The first is the catalogue
lookup at the top of `Resolve`: the URL itself is stored, so nothing is fetched or written. The
second comes after an import that collapsed onto a posting we already carry (`Result.Deduped`),
and it must NOT return early — the contribution is recorded first, because a storefront fronting
a board nobody has contributed is exactly the case where the vacancy is old and the board is new.

**Whether we know the COMPANY is a separate question, answered by `company_slug`**, which is set
whenever the catalogue already carries that employer through ANY source. The board checks cannot
answer it: `BoardTracked` is keyed by `(source, board)`, so a company we reach through a second
ATS — or through an ATS the recogniser does not know — is board-new and company-old at once.
Collapsing the two is what once told a contributor of a Dropbox posting on `dropbox.jobs` (a
Phenom vanity domain, deliberately unrecognisable) that "this company is new to us — we'll start
crawling its board", while we had been crawling Dropbox's Greenhouse board all along and no crawl
followed.

**The board `Inspect` resolves is handed to the import** (`linkimport.Board`). It overrides only
the generic resolver, which files a page under `(weblink, <the URL>)` — correct when nothing
better is known, a duplicate when the posting is one we crawl under `(greenhouse, <board>:<id>)`.
The id-in-the-URL lookup is often the only thing that knows this, since a storefront's host
names no board.

Two orderings inside it are load-bearing, both pinned by tests:
- the catalog lookup runs FIRST, or a posting we carry from an aggregator gets a second row
  under `weblink`;
- the board is inspected BEFORE the import (`Service.Inspect`), because the import writes a
  posting under that very board — asking afterwards reports every freshly imported board as
  already tracked.

The service surface is `Inspect` + `RecordIntake`; callers that do not import first compose
the two themselves.


## Limitations
- Credits are awarded before the board is verified to fetch (no network on submit). Onboarding
  the board into `sources` and any claw-back for an unreachable board are deferred to a
  background ingest worker; the `status` column keeps that option open.
- Coverage is the ~37 multi-tenant ATS in the `atsBoards` table — path-, subdomain-, host-,
  and hostpath-based. The long tail is a follow-up (one `atsBoards`-style rule + test each).
