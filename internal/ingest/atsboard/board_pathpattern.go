package atsboard

import "regexp"

// pathTenantSubdomains names the path-mode hosts that ALSO serve a tenant on its own subdomain
// with the same board id: Manatal answers <tenant>.careers-page.com beside
// careers-page.com/<tenant>, and its posting links there (/job/<hash>, /jobs/<uuid>) would
// otherwise be read as a board called "job".
var pathTenantSubdomains = map[string]bool{"careers-page.com": true}

// pathPattern is one pathPatternBoards entry; see there.
type pathPattern struct {
	path, reject, paramPattern  *regexp.Regexp
	param, listing              string
	hostBoard, tenantHost, fold bool
	// exactHost pins the entry to the apex host itself: matchHost suffix-matches every
	// subdomain of a dotted apex, and an entry whose pattern reads the WHOLE host as one
	// company's board (jobs.dover.com) must not fire for a tenant-looking subdomain —
	// foo.jobs.dover.com is not Dover's board host, and recognizing it would mint a board out
	// of a hostname this platform never serves.
	exactHost bool
}

// pathPatternBoards holds, per modePathPattern host entry, the path pattern whose one capture
// group is the board, and the board listing's path (%s = the captured value) the canonical
// collapses to. The capture is kept VERBATIM: these platforms serve any case, and the catalogue
// stores what the link it was harvested from spelled (NEOGOV's "ALBANYOR" beside "champlin",
// Paycom's upper- and lower-case client keys), so folding would miss the boards stored the other way.
//
//   - reject names captured values that are the platform's own pages, for the words a shape
//     cannot exclude on its own.
//   - hostBoard prefixes the board with the entry's host, for an adapter whose board names the
//     domain too (NEOGOV: "<domain>/<agency>", two domains being separate tenant spaces).
//   - tenantHost prefixes the board with the link's OWN host instead, for an adapter whose board
//     names the tenant's host (Taleo's "<tenant>.taleo.net/<section>", Oracle's
//     "<pod host>/<site>"): the entry's host is only the domain those hosts share.
//   - param names a query parameter that carries the board on the paths the pattern does not
//     match, held to paramPattern after folding to lower case.
//   - fold lower-cases the capture: the one exception to the verbatim rule, for a platform whose
//     catalogue holds every board lower-case.
//
// doverBoardPath is jobs.dover.com's board shape: "/<Org slug>/<posting uuid>" with at most
// one optional trailing slug segment. The slug gate is [A-Za-z0-9][A-Za-z0-9-]* (no empty
// segment, no leading/trailing-hyphen fuzziness worth honoring), the uuid gate is the exact
// canonical 8-4-4-4-12 hex spelling — the same shape atsboard's own guid-style gates use
// elsewhere — and the pattern is anchored at BOTH ends: a deeper path (/…/<uuid>/a/b) is not
// a board link the platform serves, so it is declined rather than captured with junk. The
// capture is the SLUG alone (see the host row and the jobs.dover.com entry below).
var doverBoardPath = regexp.MustCompile(`^/([A-Za-z0-9][A-Za-z0-9-]*)/[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}(?:/[A-Za-z0-9][A-Za-z0-9-]*)?$`)

var pathPatternBoards = map[string]pathPattern{
	// Dover's public job-board host (see the host row above): the BOARD is the first segment
	// alone — the company's Dover slug, the same spelling the app.dover.com apply and listing
	// paths use — captured only when a UUID-shaped second segment proves the link names a
	// board. The uuid itself is NOT the board and NOT assumed to be the client id: on the
	// submitted link it is a posting id (careers-page-slug/Deepspatial → client 1aeeba55-…;
	// careers-page/a5546acf-…/jobs 404s; both verified live 2026-11-05), so folding it in would
	// mint one board per posting and strand the crawl under an id the API does not resolve.
	// What the shell cannot prove is the slug's EXISTENCE — it answers any slug — so existence
	// is verified where it is provable, at the API, exactly as every other candidate board is.
	// A bare /<Org> and the vendor's /startup-jobs listing carry no UUID segment and are
	// declined by the pattern; a trailing posting slug is left out of the capture, and the
	// canonical collapses to the /<Org> board address, so a posting link and its listing map to
	// one board.
	"jobs.dover.com": {
		path:      doverBoardPath,
		listing:   "/%s",
		exactHost: true,
	},
	// ClearCompany, which has served HRM Direct's tenants since ~2026-10-21
	// (internal/ingest/sources/hrmdirect.go): the career page is /<board>, a posting is that page
	// with ?jobId=<guid>, and the old <board>.hrmdirect.com redirects through /<board>/redirect.
	// Other paths on the host (/<board>/jobs, /careers) answer 404. The platform answers any case
	// of the slug and all 1,515 hrmdirect boards in the catalogue are lower-case (measured
	// 2026-10-30), so the capture folds. /careers has the career page's shape, so it is rejected
	// by name.
	"apply.clearcompany.com": {
		path:    regexp.MustCompile(`^/([A-Za-z0-9][A-Za-z0-9-]*)(?:/redirect)?/?$`),
		reject:  regexp.MustCompile(`^careers$`),
		listing: "/%s",
		fold:    true,
	},
	// Paycom: a portal is /v4/ats/web.php/portal/<32-hex client key>/…
	// (internal/ingest/sources/paycom.go). An older posting link
	// (…/jobs/ViewJobDetails?job=<id>&clientkey=<key>, sometimes on a regional host) names the
	// key as a parameter instead, usually upper-case; it redirects to the portal and the
	// platform serves either case, so it is folded to the lower case most of the catalogue holds.
	"paycomonline.net": {
		path:         regexp.MustCompile(`^/v4/ats/web\.php/portal/([0-9A-Fa-f]{32})(?:/|$)`),
		param:        "clientkey",
		paramPattern: regexp.MustCompile(`^[0-9a-f]{32}$`),
		listing:      "/v4/ats/web.php/portal/%s/career-page",
	},
	// Comeet: the board is "<slug>/<company uid>" (internal/ingest/sources/comeet.go), both halves
	// required. The uid's fixed shape ("B1.001", "32.00E") is what separates a board from the
	// vendor's own pages under the same host.
	"comeet.com": comeetPattern,
	"comeet.co":  comeetPattern,
	// NEOGOV: /careers/<agency>[/jobs/<id>/<title>] on either domain. /careers/home is the
	// platform's search page, not an agency. A posting outside an agency (/jobs/<id>) names none.
	"governmentjobs.com": neogovPattern,
	"schooljobs.com":     neogovPattern,
	// GeekHunter: /<language>/<board>/jobs[/<posting slug>] (internal/ingest/sources/geekhunter.go).
	// The trailing "jobs" is what proves the segment is a company; the site's other pages under a
	// language prefix carry none.
	"geekhunter.com": {
		path:    regexp.MustCompile(`^/(?:pt|en|es)/([a-z0-9-]+)/jobs(?:/|$)`),
		listing: "/pt/%s/jobs",
	},
	// CleverStaff: a tenant's public page is /i/<alias>, which the app also answers as
	// /i/<alias>-vacancies (it strips the suffix to find the alias) and /i/<alias>/description.
	// The other single-segment routes under /i/ are the app's own; a posting (/i/vacancy-<id>)
	// names no alias at all, only the posting's short id.
	"cleverstaff.net": {
		path:    regexp.MustCompile(`^/i/([a-z0-9-]+?)(?:-vacancies)?(?:/description)?/?$`),
		reject:  regexp.MustCompile(`^(?:signup|hidden-vacancy|extension-removed|account-not-found|vacancy-.*)$`),
		listing: "/i/%s",
	},
	// Flatchr: a careers site is /<lang>/company/<board>[/vacancy/<slug>/], and the app reads it
	// from /company/<board>.json (internal/ingest/sources/flatchr.go). The site's other pages
	// (/fr/, /fr/404-not-found) carry no "company" segment.
	"careers.flatchr.io": {
		path:    regexp.MustCompile(`^/(?:[a-z]{2}/)?company/([a-z0-9][a-z0-9-]*?)(?:\.json)?(?:/|$)`),
		listing: "/fr/company/%s",
	},
	// Beehire: the career page is /career/<slug> (also /careerrss/<slug>) and the adapter reads
	// /users/getPublicCampaigns/<slug> (internal/ingest/sources/beehire.go). An invite link
	// (/invite/<key>) names a posting, not a board. The catalogue stores the slug lower-case.
	"app.beehire.com": {
		path:    regexp.MustCompile(`^/(?:career|careerrss)/([A-Za-z0-9][A-Za-z0-9_-]{0,127})/?$`),
		listing: "/career/%s",
		fold:    true,
	},
	// Inploi: a company page is /company/<slug> and redirects to that employer's career host.
	// The adapter reads /company/<slug>/search (internal/ingest/sources/inploi.go). A job URL
	// under the same prefix is the same board. The marketing site has no /company/ segment.
	"inploi.com": {
		path:    regexp.MustCompile(`^/company/([A-Za-z0-9][A-Za-z0-9-]{0,127})(?:/|$)`),
		listing: "/company/%s",
		fold:    true,
	},
	// TriNet Hire: /companies/<numeric id>[-<slug>]/jobs[/<posting>] (internal/ingest/sources/
	// trinethire.go). The leading id is what proves the segment is a company; the host's own
	// pages (/privacy) sit outside /companies/.
	"app.trinethire.com": {
		path:    regexp.MustCompile(`^/companies/(\d+(?:-[a-z0-9-]+)?)(?:/|$)`),
		listing: "/companies/%s/jobs",
	},
	// Velora HR: /<slug>-<career-site id>[/offers/<offer id>] on <tenant>.career.velorahr.com
	// (internal/ingest/sources/velorahr.go). The board is the trailing id alone: the site renders
	// whatever slug precedes it (/158 and /wetaca-158 are one page), and the tenant host names no
	// site, since one tenant may run several. The host's other pages carry no trailing number.
	"career.velorahr.com": {
		path:    regexp.MustCompile(`^/(?:[A-Za-z0-9-]*-)?([1-9][0-9]{0,8})(?:/|$)`),
		listing: "/%s",
	},
	// GoHire: /<slug>-<8-character site code>/[<posting>/] (internal/ingest/sources/gohire.go).
	// The code's shape is what separates a site from the platform's /assets and from the
	// retired numeric form (dexerto-19265), which answers 404.
	"jobs.gohire.io": {
		path:    regexp.MustCompile(`^/([a-z0-9][a-z0-9-]*-[a-z0-9]{8})(?:/|$)`),
		listing: "/%s/",
	},
	// Taleo Enterprise on the vendor's own domain: <tenant>.taleo.net/careersection/<section>/
	// <page>.ftl, and the adapter's board is "<host>/<section>" (internal/ingest/sources/taleo.go).
	// A section is named freely ("2", "ex", ".nexcom_ext_prof_cs", "rstrcs1.1"); reject names the
	// platform's own trees beside them (/careersection/rest/…, /careersection/iam/…). A Taleo career site on the employer's own domain stays underivable.
	"taleo.net": {
		path:       regexp.MustCompile(`^/careersection/([A-Za-z0-9_.-]+)(?:/|$)`),
		reject:     regexp.MustCompile(`^(?:(?i:rest|iam)|\.+)$`),
		listing:    "/careersection/%s/jobsearch.ftl",
		tenantHost: true,
	},
	// Oracle Recruiting Cloud on the vendor's pod hosts (<tenant>.fa.<pod>.oraclecloud.com):
	// /hcmUI/CandidateExperience/<lang>/sites/<site>/…, board "<host>/<site>"
	// (internal/ingest/sources/oracle.go). The fixed words match in any case (links spell them
	// both ways); the site keeps its own, since the REST finder takes it as spelled.
	"oraclecloud.com": {
		path:       regexp.MustCompile(`^/(?i:hcmui/candidateexperience/[a-z_-]+/sites)/([A-Za-z0-9_]+)(?:/|$)`),
		listing:    "/hcmUI/CandidateExperience/en/sites/%s",
		tenantHost: true,
	},
	// Loxo agency boards: <host>/<slug>, the host being app.loxo.co, a regional pod or the
	// agency's own label under it, and the board "<host>/<slug>" (internal/ingest/sources/loxo.go).
	// A posting (/job/<token>) names no agency, and the app's own single-segment routes are
	// rejected by name.
	"app.loxo.co": {
		path:       regexp.MustCompile(`^/([A-Za-z0-9][A-Za-z0-9_-]*)/?$`),
		reject:     regexp.MustCompile(`^(?i:job|jobs|login|logout|signin|signup|sign_in|sign_up|register|users|password|dashboard|settings|admin|api|help|support|privacy|terms)$`),
		listing:    "/%s",
		tenantHost: true,
	},
	// RecruiterFlow agency career pages: recruiterflow.com/<board>/jobs[/<posting id>]
	// (internal/ingest/sources/recruiterflow.go). The trailing "jobs" is what proves the segment
	// is an agency; the vendor's own pages on the same host carry none.
	"recruiterflow.com": {
		path:    regexp.MustCompile(`^/([A-Za-z0-9][A-Za-z0-9_-]*)/jobs(?:/|$)`),
		listing: "/%s/jobs",
	},
}
