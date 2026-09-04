# ATS board recognition conventions

## Scope
Turning a job URL into the company board it belongs to — `(source, board, canonical)` — using
nothing but the URL. No network, no database.

## Always true
- **One definition, three consumers.** `internal/ingest/contribution` uses it to decide which board a
  pasted link would onboard, `internal/ingest/linksource` (board coverage) uses it to find the ingest
  adapter that can fetch that board, and `internal/ingest/boardresolve` uses it to identify an ATS
  embedded in a company's own careers page. It lives here so a host added once is recognised by
  all three; it used to live in `contribution`, which `boardresolve` already reached across for.
- **The `source` MUST be the provider key the catalogue uses** — the string an ingest adapter's
  `Provider()` returns, not a name derived from the domain. This is load-bearing in a way that
  fails silently: the board-tracked check looks jobs up by `(source, board)`, so a board we
  already crawl under one name looks brand new under another — it is recorded as a fresh
  contribution and paid for, and board coverage finds no adapter to read it with. Factorial was
  exactly this: `<tenant>.factorialhr.<tld>` mapped to a `factorialhr` source that does not
  exist, while one adapter serves both domains as `factorial`.
- **Adding an ATS is one row plus one test case.** Extraction modes say where the tenant sits:
  `path` (first path segment), `pathlocale` (same, skipping a leading `xx-XX` locale),
  `pathlocalepair` (the first TWO segments after that locale — Dayforce's board is
  `<tenant>/<site>` and one tenant runs several sites), `pathportal` (the segment before the
  posting, for SmartRecruiters' portal URLs — plus its
  one-click apply form, `/oneclick-ui/company/<board>/publication/<uuid>`, which names the
  employer mid-path and would otherwise be read as a board called `oneclick-ui`),
  `pathnumeric` (like `path`, but the segment must be an all-digit id — PageUp's board is a
  numeric institution id, so localisation/section segments like `/cw/en/search` are not
  boards), `query` (a named query parameter, because Paycor serves every board from one path
  under `?clientId=<board>`; `queryBoards` names the parameter and the shape its value must
  have, since a parameter is a weaker signal than a path segment — and it is the one mode that
  needs a second row, so a test fails if it is missing), `subdomain`
  (leftmost DNS label), `subdomainchain` (every label under the apex, for a tenant nested under a
  regional instance like `<tenant>.global.huntflow.io`), `host` (the whole careers host IS the
  tenant), `hostpath` (host + first path segment, for Workday), `hostcareers`
  (`<host>/<tenant>` from `<host>/ta/<tenant>.careers`, for UKG Ready, whose host selects the
  environment its tenant lives in), `hosttenantboard` (`<host>/<tenant>/<guid>`, for UKG Pro
  Recruiting — a different product from UKG Ready, on different hosts).
- **The mode must match how the ingest adapter addresses the board**, not how the URL reads. The
  board string is copied verbatim into the `boards` catalog and into the `external_id`
  namespace, so a truncated one is a board that 404s every crawl: Huntflow's adapter fetches
  `<board>.huntflow.io`, which is why a regional tenant's board keeps its `.global` label.
- **Fail-safe by construction.** A wrong or missing entry makes a link *unrecognised*, never a
  false board: a bad apex or mode yields an empty board, which is declined. So a best-guess
  host is safe to add.
- **A white-labelled ATS is one row per reseller domain.** HiringThing serves one application
  from ~25 domains and its board IS the careers host, so a slug names nothing on its own — the
  rows are copied from the domains the catalog's `hiringthing` boards are actually on, and an unlisted one
  is simply unrecognised.
- **A platform's own hosts are declined** (`platformHost`). In `host` and `subdomain` mode the
  host IS the board, so a vendor's console (`app.teamtailor.com`, which every Teamtailor career
  site links to) would otherwise be recorded as an employer. `platformLabels` holds the labels
  no ATS gives a tenant; `platformLabelsByApex` holds the ones only ONE platform keeps —
  `apply` is HRM Direct's application host **and** a recruitee board we crawl, so declining it
  everywhere would drop a real board.

## Limitations
- **Vanity domains are invisible here.** Recognition keys on host, so a supported ATS behind a
  company's own domain (`careers.peraton.com` on iCIMS, `jobs.ea.com` on Avature) yields
  nothing — the largest category in the review queue. `internal/ingest/boardresolve` fetches the page
  and detects the embedded board; this package stays network-free.
- Custom-domain ATS (Taleo, SuccessFactors, Oracle, Workday tenants on their own domain) are
  absent on purpose: their board cannot be derived from a URL at all.
