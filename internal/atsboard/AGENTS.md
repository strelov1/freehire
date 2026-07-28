# ATS board recognition conventions

## Scope
Turning a job URL into the company board it belongs to — `(source, board, canonical)` — using
nothing but the URL. No network, no database.

## Always true
- **One definition, three consumers.** `internal/contribution` uses it to decide which board a
  pasted link would onboard, `internal/linksource` (board coverage) uses it to find the ingest
  adapter that can fetch that board, and `internal/boardresolve` uses it to identify an ATS
  embedded in a company's own careers page. It lives here so a host added once is recognised by
  all three; it used to live in `contribution`, which `boardresolve` already reached across for.
- **The `source` MUST be the provider key the catalogue uses** — the string an ingest adapter's
  `Provider()` returns, not a name derived from the domain. This is load-bearing in a way that
  fails silently: the board-tracked check looks jobs up by `(source, board)`, so a board we
  already crawl under one name looks brand new under another — it is recorded as a fresh
  contribution and paid for, and board coverage finds no adapter to read it with. Factorial was
  exactly this: `<tenant>.factorialhr.<tld>` mapped to a `factorialhr` source that does not
  exist, while one adapter serves both domains as `factorial`.
- **Adding an ATS is one row plus one test case.** Five extraction modes say where the tenant
  sits: `path` (first path segment), `pathlocale` (same, skipping a leading `xx-XX` locale),
  `subdomain` (leftmost DNS label), `host` (the whole careers host IS the tenant), `hostpath`
  (host + first path segment, for Workday).
- **Fail-safe by construction.** A wrong or missing entry makes a link *unrecognised*, never a
  false board: a bad apex or mode yields an empty board, which is declined. So a best-guess
  host is safe to add.
- **A platform's own hosts are declined** (`platformHost`). In `host` and `subdomain` mode the
  host IS the board, so a vendor's console (`app.teamtailor.com`, which every Teamtailor career
  site links to) would otherwise be recorded as an employer.

## Limitations
- **Vanity domains are invisible here.** Recognition keys on host, so a supported ATS behind a
  company's own domain (`careers.peraton.com` on iCIMS, `jobs.ea.com` on Avature) yields
  nothing — the largest category in the review queue. `internal/boardresolve` fetches the page
  and detects the embedded board; this package stays network-free.
- Custom-domain ATS (Taleo, SuccessFactors, Oracle, Workday tenants on their own domain) are
  absent on purpose: their board cannot be derived from a URL at all.
