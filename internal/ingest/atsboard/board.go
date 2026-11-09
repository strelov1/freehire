// Package atsboard recognises which company board a job URL belongs to, using nothing but
// the URL: a host maps to an ATS provider and an extraction mode, and the mode says where
// in the URL the tenant's board slug sits.
//
// It is deliberately a small local table rather than a method on each ATS adapter —
// adding a platform is one row plus one test case. It is also deliberately shared: the
// contribution flow uses it to decide which board a pasted link would onboard, link
// resolution uses it to find the ingest adapter that can fetch that board, and
// boardresolve uses it to identify an ATS embedded in a company's own careers page. One
// definition means a host added once is recognised by all three.
//
// The source a host maps to MUST be the provider key the catalogue uses — the string an
// ingest adapter's Provider() returns. Deriving it from the domain instead fails silently
// and expensively: the board-tracked check looks jobs up by (source, board), so a board we
// already crawl under one name looks brand new under another, is recorded as a fresh
// contribution, and is paid for.
package atsboard

import (
	"slices"
)

// Board extraction modes, each matching how the ingest adapter namespaces jobs.external_id:
//   - path:      board = the first path segment on a fixed host (jobs.lever.co/<board>/…).
//   - pathlocale: like path, but a leading xx-XX locale segment is skipped first — Rippling's
//     public site prefixes the board with a locale (ats.rippling.com/en-GB/<board>/…) that its
//     board API omits, so both URL shapes must resolve to the same board.
//   - pathportal: board = the segment before the posting segment, because SmartRecruiters serves
//     the same posting both bare (<company>/<posting>) and behind a portal segment
//     (<portal>/<company>/<posting>). Taking the first segment reads the portal slug as a board.
//   - subdomain: board = the leftmost DNS label under a fixed apex (<board>.recruitee.com).
//   - subdomainchain: board = EVERY label under the apex, because the platform nests a tenant
//     under a regional instance — Huntflow serves its international tenants at
//     <tenant>.global.huntflow.io and its adapter fetches <board>.huntflow.io, so the board is
//     "<tenant>.global"; taking the leftmost label alone yields a host that 404s.
//   - host:      board = the whole careers host (the tenant identity IS the host, and the TLD
//     varies by region, e.g. <tenant>.zohorecruit.eu / .com / .in).
//   - hostpath:  board = "<host>/<first path segment>" (Workday: the tenant is the host, the
//     site is the first path segment, e.g. acme.wd1.myworkdayjobs.com/Careers).
//
// For subdomain, subdomainchain and host the board IS the host; for hostpath it is host + site.
// In all these the canonical URL is stripped to that board, collapsing a vacancy URL and the
// board listing to one.
const (
	modePath           = "path"
	modePathLocale     = "pathlocale"
	modePathPortal     = "pathportal"
	modeSubdomain      = "subdomain"
	modeSubdomainChain = "subdomainchain"
	modeHost           = "host"
	modeHostPath       = "hostpath"
	// pathnumeric: like path, but the segment must be an all-digit id. PageUp's board is a
	// numeric institution id, so its localisation and section segments (/cw/en/search) are not
	// boards — reading one as a board names an institution that does not exist.
	modePathNumeric = "pathnumeric"
	// hosttenantboard: board = "<host>/<tenant>/<guid>" from <host>/<tenant>/JobBoard/<guid>/….
	// UKG addresses a board by all three: the host carries the data-residency TLD, the tenant is
	// its customer code and the guid picks one of the tenant's boards. The adapter needs the same
	// three (internal/ingest/sources/ukg.go), so anything less is not a shorter board id — it is one the
	// crawl rejects outright.
	modeHostTenantBoard = "hosttenantboard"
	// pathlocalepair: like pathlocale, but the board is the first TWO segments after the optional
	// locale. Dayforce serves every career site from one host under "<culture?>/<tenant>/<site>"
	// and a board is "<tenant>/<site>": one tenant may run several sites and each is its own
	// board, so the tenant alone is not a shorter board id but one the crawl rejects. The culture
	// stays out — a posting keeps the same id in every culture it is translated into, which is
	// why the ingest adapter folds it off too (sources.boardIdentity).
	modePathLocalePair = "pathlocalepair"
	// query: board = a named query parameter, because the platform addresses a tenant by
	// parameter rather than by path or host — Paycor serves every board from one path under
	// "?clientId=<board>". queryBoards names the parameter per host, and the listing path the
	// canonical collapses to.
	modeQuery = "query"
	// hostcareers: board = "<host>/<tenant>" from <host>/ta/<tenant>.careers. UKG Ready's host
	// selects the environment its tenant is hosted in and is not derivable from the tenant id, so
	// the adapter needs both (internal/ingest/sources/ukgready.go) — the same self-describing shape
	// hostpath and hosttenantboard carry. It is named after the career-page path rather than after
	// its parts, so it cannot be misread as a variant of hosttenantboard above: that one is UKG
	// PRO Recruiting, a different product on different hosts with a different board shape.
	modeHostCareers = "hostcareers"
	// pathpair: board = the first TWO path segments, verbatim, no folding. werecruit serves every
	// career site from one host under "<locale>/<tenant>/…" and the locale is REQUIRED and part
	// of the board's identity — a tenant not configured for a locale answers it with an empty
	// listing rather than an error, so unlike pathlocalepair's optional-and-dropped locale, there
	// is nothing here to fold off.
	modePathPair = "pathpair"
	// icims: board = a bare iCIMS slug OR a full vanity host, mirroring the ingest adapter's own
	// split (internal/ingest/sources/icims.go, icimsVanity/icimsHost): the classic two-label
	// shape "careers-<slug>.icims.com" folds to the bare slug, and every other *.icims.com host
	// (a client's own vanity subdomain, e.g. uscareers-lennox.icims.com) IS the board, kept
	// whole — the same "board contains a dot" signal the adapter itself reads to tell the two
	// apart, so there is nothing for the recognizer to decide beyond matching that one shape.
	modeICIMS = "icims"
	// querypair: board = TWO named query parameters joined by ":", because the platform needs
	// both to address a tenant and neither alone identifies one. ADP Workforce Now serves every
	// career centre from one path under "?cid=<tenant>&ccId=<centre>", and its ingest adapter
	// takes the board as exactly that pair (internal/ingest/sources/adp.go splits on the colon),
	// so a single parameter would be a board that 404s every crawl. Taleo Business Edition is the
	// same shape ("?org=<ORG>&cws=<site>"). queryPairBoards names the two
	// parameters and the shape each value must have, for the reason modeQuery demands one: on a
	// board-only host a parameter is a weak signal, and inventing a board is the expensive
	// direction.
	modeQueryPair = "querypair"
	// allowedfirstsegment: board = the segment right AFTER one of a small ALLOWED set of leading
	// path segments (see allowedFirstSegments) — the inverse of reservedSegments' deny list. It
	// exists for a host that is a platform's WHOLE product surface rather than a dedicated boards
	// subdomain: Dover serves its dashboard, marketing pages, and sign-in from the same
	// app.dover.com a board's /apply/<slug>/<jobId> and /jobs/<slug> also live on, so reading every
	// unrecognized leading segment as a tenant (the ordinary path mode) would mint a board out of
	// "/pricing" or "/login". Any first segment not on the allow list is declined outright.
	modeAllowedFirstSegment = "allowedfirstsegment"
	// tenanthost: board = the whole host, recognised only when it sits exactly ONE label under the
	// platform's shared domain (<tenant>.jobs.feishu.cn, <tenant>.phenompro.com). The adapter
	// addresses a tenant by its full careers host, like host mode's, but the shared domain here
	// is the host's suffix rather than a label in its middle, which host mode's match cannot
	// express. A deeper host is declined rather than read: nothing says it is a tenant. When
	// tenantHostPostingSegments names a path segment for the apex, a URL must carry it — for a
	// vendor that serves its own machinery (a CDN, an API) from the same domain its tenants live
	// on, which a career page links to on every load.
	modeTenantHost = "tenanthost"
	// keyedsegment: board = the segment right after a fixed key segment, and only when it has the
	// platform's board-id shape (keyedSegmentBoards). Workstream addresses an employer at
	// /j/<8-hex>/…, but the same /j/ also takes the employer's SLUG, which 301-redirects onto the
	// hex — a slug is a spelling of the board, not the id the adapter namespaces postings by, so
	// reading it would file the employer under a board no crawl stores.
	modeKeyedSegment = "keyedsegment"
	// pathjob: board = the first path segment, but ONLY on a posting URL — "<board>/job/<id>-…"
	// with a numeric id — and only on the apex host itself. Harri serves its marketing pages,
	// sign-in and search from the same harri.com its career pages (harri.com/<brand slug>) live
	// on, and its other hosts (gateway., solutions., resources.) are machinery, so neither a
	// bare first segment nor a subdomain match is evidence of a brand. The platform matches the
	// slug case-blind; it is lower-cased here, the spelling the catalogue stores.
	modePathJob = "pathjob"
	// encodedcareers: board = the decimal id a career site's path carries base64-encoded, from
	// "/<slug>/<base64 id>/careers". IdealTraits addresses an employer by its numeric business id
	// and spells it that way in the career-site URL; the slug is decoration (a wrong one 301s to
	// the right one), and the adapter (internal/ingest/sources/idealtraits.go) takes the decimal
	// id. A posting link ("/career/<slug>/<posting id>") carries no business id at all and is
	// declined — boardresolve reaches the board through the career-site link every posting page
	// carries.
	modeEncodedCareers = "encodedcareers"
	// hostsite: board = "<host>/<site>" from <host>/co/<site>/…, lower-cased. Paradox serves
	// every career site under "/co/<site>" on one of several instance hosts — regional
	// (olivia.paradox.ai, olivia.eu1.paradox.ai) and per-customer (darden.paradox.ai) — and a
	// site exists on exactly one of them, so the adapter needs both
	// (internal/ingest/sources/paradox.go). Site slugs are case-insensitive at the platform.
	modeHostSite = "hostsite"
	// pathpattern: board = the one capture group of a per-host path pattern (pathPatternBoards),
	// for a platform whose board sits at a fixed position behind fixed words on a host that also
	// serves the vendor's own pages — Comeet's /jobs/<slug>/<uid>, NEOGOV's /careers/<agency>,
	// Paycom's /v4/ats/web.php/portal/<key>. The pattern carries the board's SHAPE as well as its
	// position, so a vendor page that merely shares the leading word matches nothing. A path the
	// pattern does not match is declined, never read some other way.
	modePathPattern = "pathpattern"
)

// atsBoards lists the supported multi-tenant ATS: a host (exact or subdomain-suffix match) →
// its source key and extraction mode. Hosts were verified against each adapter's public job
// URL. A wrong/missing entry is fail-safe: the link simply isn't recognized (422), never a
// false board. Single-company brands, aggregators, and custom-domain ATS (Taleo, SuccessFactors,
// Oracle, and Workday tenants on their own domain) are absent — their board can't be derived
// from a URL. The vendor-hosted forms ARE derivable: Workday's *.myworkdayjobs.com (host + site),
// *.taleo.net and *.oraclecloud.com (see pathPatternBoards). Workday's shared
// wdN.myworkdaysite.com host is left out on purpose: the same site also answers on the tenant's
// own <tenant>.wdN.myworkdayjobs.com, which is how the catalogue mostly stores it, so reading
// the shared spelling would file a board we already crawl as a new one.
var atsBoards = slices.Concat(atsBoardsA, atsBoardsB)
