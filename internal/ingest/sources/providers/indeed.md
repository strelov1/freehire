**Indeed traps** (`indeed`; public pages sampled 2026-11-24 through the crawl host):

- **Only the crawl host's own browser is served.** Plain and fingerprinted HTTP get Cloudflare's
  "Security Check" (403), Scrapling `http` + proxy gets 401 "Authenticating...", and the
  `browser` + proxy route timed out. `browser` without the proxy reads the search page: ~110 s
  for the challenge on the first page of a host (each country site is its own host), ~7 s a page
  after that on the warm session.
- **A guest sees one search page, 15 cards.** Page two (`start=10`) and `/viewjob?jk=` redirect
  to `secure.indeed.com/auth` (`from=page-two-signin`, `from=bot-detection-anonymous`). The
  adapter treats ending on `/auth` as a refusal: it stops the board and keeps what it read.
- **The body comes from the search page, not the posting page.** The cards
  (`window.mosaic.providerData["mosaic-provider-jobcards"]`) carry title, employer, location,
  country, salary, job types and `remoteWorkModel`, but only a snippet. The page also opens ONE
  posting in its right pane (`window._initialData.autoOpenTwoPaneViewjobResponse.body`, with
  `sanitizedJobDescription`), and `vjk=<jobkey>` chooses which. A new card therefore costs one
  navigation to the same search with its key selected; the listing page pays for its first card.
  Both the final URL's `vjk` and the opened `jobKey` must equal the key asked for.
- Board = keyword (`q`, required), Region = location (`l`, empty = whole country).
  `tenants.site` = `www` (US, default) or a two-letter subdomain (`de`, `uk`, `ca`);
  `tenants.fromage` = 1, 3, 7 or 14 days (default 1). Anything else fails before a request.
  Register with `cmd/add-board --provider=indeed --board='software engineer' --region='Remote'
  --company='Indeed US software' --apply` (tenants for another country).
- **Identity is the global job key** (16 hex digits), stored un-namespaced like LinkedIn's
  posting id (`pipeline.jobIdentity`, `seenScope`), so two keywords that find one posting make
  one row and the seen-set is provider-wide.
- **Nothing can probe a posting**, and a live one drops out of its search's 15 cards within a
  day or two. So no board-scoped close: `SweepGrace` is 30 days and `cmd/liveness`'s age rule
  (`expireDespiteRegisteredPrefixes`) closes what has gone unseen that long.
- `indeedApplyable` cards are `ApplyOnSite` (Indeed Apply); the rest apply on the employer's
  site, behind `/rc/clk` redirects robots.txt disallows, so the stored URL is the `/viewjob` page.
- One navigation every 3 s across all boards (`indeedRequestInterval`); a search with 15 new
  cards takes ~2 minutes.
