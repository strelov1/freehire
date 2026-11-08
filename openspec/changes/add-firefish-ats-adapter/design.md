## Context

Firefish (firefishsoftware.com) is a UK recruitment CRM whose tenants' career sites live under
two spellings: the tenant's own domain (`careers.arnoldclark.com`) and a subdomain of the
vendor's (`arnoldclark.firefishsoftware.com`, which serves the SAME site — its channel `<link>`
points back to the branded domain). There is no JSON API; the machine-readable surface is the
RSS document every site publishes at `/rss/adverts/latest.aspx`, an RSS 2.0 channel whose items
carry the whole posting:

- `<title>`, `<description>` (full HTML body inside CDATA), `<link>`/`<guid>` (the tenant's
  canonical posting URL, `…/job/<slug>-<reference>.aspx`);
- `ffAdvert:` extension elements (namespace `http://www.firefishsoftware.com/`):
  `ReferenceNumber` (the vacancy's own id, stable across title edits), `JobType` ("Permanent",
  "Contract"), `Discipline`/`Role`/`Specialisation`, `LocationArea`, `PostedDate`/`ClosingDate`
  (RFC1123 GMT), `Country` ("Scotland", "England", "United Kingdom", or empty),
  `MinimumPayment`/`MaximumPayment` (0.00 = not stated), `PaymentRate` ("Per Year", "Per Day"),
  `Currency`, `Remuneration` (prose).

One unpaginated GET returns the complete open-vacancy list: measured live 2026-11-07,
`careers.arnoldclark.com` serves 342 vacancies in one ~1.1 MB document ( postings with years of
history — the reference numbers run up to 92xxx), and a site with no openings answers a
well-formed, item-less channel. An unknown tenant label NXDOMAINs; the vendor's own careers
site (`jobs.firefishsoftware.com`) is itself a tenant-shaped site with its own feed.

Existing conventions this design follows (see `internal/ingest/sources/AGENTS.md`,
`internal/ingest/atsboard/AGENTS.md`): the RSS-feed adapters `gethired.go` and `taleobe.go`
(one feed per board, fullBoardListing), and the `Job` struct (`internal/ingest/sources/source.go`).

## Goals / Non-Goals

**Goals:**
- Crawl any Firefish-hosted career site into the catalogue with one request per board, the
  whole posting from the feed.
- Recognize `<tenant>.firefishsoftware.com` URLs for board-contribution intake without
  misreading the vendor's own hosts.

**Non-Goals:**
- Discovering Firefish tenants automatically (no tenant directory; boards are added one at a
  time via `board_submissions`/`cmd/add-board`, the tenant's careers host being the board id).
- Crawling the vendor's own jobs site as a board of its own (it is declined as a platform host;
  it can be added explicitly like any single-tenant host if ever wanted).
- Submitting applications through Firefish (out of scope; apply forms are vendor-internal).

## Decisions

**Adapter shape: plain list-only `Fetch`, marked `fullBoardListing`, `FetchNew` delegating to
`Fetch`.** The feed carries the whole posting — body and facets — so there is no detail stage
to skip and a re-upsert of a seen vacancy costs nothing beyond the one listing GET the board
pays anyway (the same shape `gethired.go` documents). A content-less `SeenRefresh` would be
strictly worse: the Job is already complete.

**Board id = the tenant's whole careers host (`careers.arnoldclark.com`).** The board must be
an address the adapter can fetch, and Firefish tenants are reachable by host, not by label:
the same site answers under the vendor's subdomain and the branded domain, and the branded one
is what postings link. This mirrors `intervieweb`/`eploy` (modeHost) and `hiringthing` (whole
host as board). `<tenant>.firefishsoftware.com` folds to the same board string as its branded
host would be added under — the host IS the identity either way.

**Recognizer: `modeHost` on `firefishsoftware.com`, vendor hosts declined by label.** Every
tenant is one label under the vendor's domain, so host mode derives the board the adapter
fetches and any path on the host (vacancy page, RSS feed, search) collapses to the bare host.
`jobs` and `www` go into `platformLabelsByApex` — `jobs` is the vendor's own careers site (a
tenant-shaped page no employer owns), `www` its marketing site. An unknown label NXDOMAINs
rather than serving a page, so the fail-safe direction (an unresolvable board recorded) has no
live path here; vanity tenant domains stay unrecognised like every other ATS's.

**Identity = `ffAdvert:ReferenceNumber`.** It is the vacancy's own id the URL slug embeds
(`mechanic-92656`), stable when the title is edited, and unique per site — exactly what the
pipeline namespaces by board. Items without it (or without a link) are dropped: neither a
close nor a visitor can reach them.

**Salary only from the structured fields, whole or not at all.** `MinimumPayment`/
`MaximumPayment`/`Currency`/`PaymentRate` state bounds, currency and period together; a 0.00
bound means not stated, and a salary whose rate is not recognised is dropped whole rather than
emitted bare (bounds without a unit would read as annual downstream). `Remuneration` prose is
never parsed — it is already inside the description body the feed carries.

**Employment type: explicit map, unknown → unset.** The measured corpus spells "Permanent"
(341 of 342) and "Contract" (1); the map covers the plausible family (part time, temporary,
fixed term, apprenticeship, graduate) and leaves anything else to the pipeline's dictionaries.

**ClosingDate bypasses NotFuture; PostedDate goes through `parsePubDate` (which applies it).**
A deadline legitimately sits weeks ahead — `parsePubDate`'s NotFuture guard would nil it — so
it is parsed with the same RFC1123 layouts directly, like `ess` and `intervieweb` do for their
own deadline fields. A stale one past today closes the vacancy on the write.

**Country through the shared dictionary.** The feed names constituent countries ("Scotland",
"England") and the UK; `location.NormalizeCountry` already maps all of them to `gb`. An empty
`Country` (158 of 342 on Arnold Clark) leaves the field to the pipeline's location dictionary,
which reads the `LocationArea` text.

## Risks / Trade-offs

- **One document per board can grow.** Arnold Clark's 342 vacancies are ~1.1 MB; a tenant with
  thousands of postings would serve more. The client's existing body cap bounds the read; if a
  tenant ever outgrows it, the feed is the only listing surface and the board would need
  sharding like any other single-document source. Not a 2026 problem on the measured corpus.
- **`JobType` vocabulary is measured on one tenant.** 341 of 342 items spell "Permanent"; the
  map's other branches are best-effort from UK recruitment phrasing and an unknown value
  degrades to the dictionaries, never to a wrong facet.
- **The `ffAdvert` namespace could change.** The elements are read as namespaced children of
  `<item>`; a vendor renaming them degrades to title+body-only jobs (identity survives through
  the reference number only while it is also present — dropped items would empty the listing
  and the "some candidates, none read" rule does not apply to a genuinely empty read). The
  XMLName check on `<rss>` keeps a non-feed answer from reading as an empty board.

## Migration Plan

New provider; nothing migrates. Boards are onboarded one at a time through the contribution
flow (`cmd/add-board --provider=firefish --board=careers.arnoldclark.com …`).
