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
	"net/url"
	"regexp"
	"slices"
	"strings"
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
)

// atsBoards lists the supported multi-tenant ATS: a host (exact or subdomain-suffix match) →
// its source key and extraction mode. Hosts were verified against each adapter's public job
// URL. A wrong/missing entry is fail-safe: the link simply isn't recognized (422), never a
// false board. Single-company brands, aggregators, and custom-domain ATS (Taleo, SuccessFactors,
// Oracle, and Workday tenants on their own domain) are absent — their board can't be derived
// from a URL. Workday's standard *.myworkdayjobs.com hosts ARE derivable (host + site).
var atsBoards = []struct{ host, source, mode string }{
	// --- path: board = first path segment on a fixed host ---
	{"greenhouse.io", "greenhouse", modePath},
	{"jobs.lever.co", "lever", modePath},
	{"jobs.eu.lever.co", "lever", modePath}, // Lever EU data-residency host; same path shape/board as the US host
	{"jobs.ashbyhq.com", "ashby", modePath},
	{"apply.workable.com", "workable", modePath},
	{"jobs.deel.com", "deel", modePath},
	{"jobs.gem.com", "gem", modePath},
	{"jobs.jobvite.com", "jobvite", modePath},
	{"jobs.quickin.io", "quickin", modePath},
	{"jobs.talenthr.io", "talenthr", modePath},
	{"careers.pageuppeople.com", "pageup", modePathNumeric},
	{"oportunidades.mindsight.com.br", "mindsight", modePath},
	{"careers.hireology.com", "hireology", modePath},
	// Manatal's hosted career-page domain is PATH-based (careers-page.com/<tenant>/job/<id>),
	// not a tenant subdomain, and its source is manatal: careerspage.yml is deliberately empty
	// because every careers-page.com tenant is served by the Manatal adapter.
	{"careers-page.com", "manatal", modePath},
	// Gusto Hiring serves a board at /boards/<board>, where the board is the WHOLE
	// "<company-slug>-<company-uuid>" segment — neither half resolves alone. "boards" is the
	// platform's own path word (reservedSegments), so it is skipped to reach the board behind it.
	// A /postings/<…> URL is declined (noBoardFirstSegments): its segment ends in the POSTING's
	// uuid and carries the job's title slug, so it names no board at all — the only link back to
	// one is the breadcrumb the posting page renders, which is boardresolve's job, not a URL's.
	{"jobs.gusto.com", "gusto", modePath},

	// --- pathportal: board = the segment before the posting segment ---
	{"jobs.smartrecruiters.com", "smartrecruiters", modePathPortal},
	{"careers.smartrecruiters.com", "smartrecruiters", modePathPortal},

	// --- pathlocale: like path, skipping a leading xx-XX locale segment ---
	{"ats.rippling.com", "rippling", modePathLocale},

	// --- pathlocalepair: board = the two path segments after an optional leading locale ---
	{"jobs.dayforcehcm.com", "dayforce", modePathLocalePair},

	// --- query: board = a named query parameter (see queryBoards) ---
	{"recruitingbypaycor.com", "paycor", modeQuery},

	// --- subdomain: board = leftmost DNS label under the apex ---
	{"recruitee.com", "recruitee", modeSubdomain},
	{"bamboohr.com", "bamboohr", modeSubdomain},
	{"breezy.hr", "breezy", modeSubdomain},
	{"freshteam.com", "freshteam", modeSubdomain},
	{"peopleforce.io", "peopleforce", modeSubdomain},
	{"jobs.personio.com", "personio", modeSubdomain},
	{"jobs.personio.de", "personio", modeSubdomain}, // Personio DE regional host; board = same tenant subdomain
	{"pinpointhq.com", "pinpoint", modeSubdomain},
	{"talentlyft.com", "talentlyft", modeSubdomain},
	{"traffit.com", "traffit", modeSubdomain},
	{"applytojob.com", "jazzhr", modeSubdomain},
	{"applicantpro.com", "applicantpro", modeSubdomain},
	{"isolvedhire.com", "isolvedhire", modeSubdomain},
	{"careerplug.com", "careerplug", modeSubdomain},
	{"csod.com", "cornerstone", modeSubdomain},
	{"enlizt.me", "enlizt", modeSubdomain},
	{"hurma.work", "hurma", modeSubdomain},
	{"inhire.app", "inhire", modeSubdomain},
	{"likeit.fi", "likeit", modeSubdomain},
	{"spark.work", "spark", modeSubdomain},
	{"hire.trakstar.com", "trakstar", modeSubdomain},
	{"portaldetalentos.senior.com.br", "senior", modeSubdomain},
	{"vagas.solides.com.br", "solides", modeSubdomain},
	{"softgarden.io", "softgarden", modeSubdomain},
	// softgarden's regional career host. The tenant label is the same board the adapter fetches
	// at <board>.softgarden.io, so this apex only changes where the label sits — keyed on
	// softgarden.io alone, a .de tenant hid behind a second DNS label and was declined.
	{"career.softgarden.de", "softgarden", modeSubdomain},
	{"careers.hibob.com", "hibob", modeSubdomain}, // HiBob's careers module: <tenant>.careers.hibob.com
	// HRM Direct's career site is <board>.hrmdirect.com, exactly what hrmdirect.yml stores. Its
	// application forms live on the platform's own apply.hrmdirect.com — see this apex's entry
	// in platformLabelsByApex, without which every apply link on the platform names a board.
	{"hrmdirect.com", "hrmdirect", modeSubdomain},

	// --- subdomainchain: board = every label under the apex (tenant nested under a region) ---
	{"huntflow.io", "huntflow", modeSubdomainChain},

	// --- host: board = the whole careers host (regional TLD varies) ---
	{"zohorecruit", "zohorecruit", modeHost},
	{"teamtailor", "teamtailor", modeHost}, // <tenant>.teamtailor.com; custom-domain career sites are absent (not URL-derivable)
	{"factorial", "factorial", modeHost},   // <tenant>.factorial.<tld>
	{"factorialhr", "factorial", modeHost}, // the .com.br/.pt/… base-domain variant — ONE ingest adapter serves both, and it reports "factorial"
	// CatsOne's adapter fetches https://<board>/careers, so the board is the whole host, exactly
	// as catsone.yml stores it. It was listed as a subdomain, which yields the bare label — a
	// board no crawl can resolve. (Its custom-domain tenants, e.g. jobs.evoplay.com.ua, stay
	// underivable from a URL, like every other custom-domain ATS here.)
	{"catsone", "catsone", modeHost},
	// Avature's board is the career-site host, the shape avature.yml stores
	// (deloittecm.avature.net). Only tenants on the platform's own domain are derivable; a
	// vanity host (jobs.ea.com) is not, like every other custom-domain ATS here.
	{"avature", "avature", modeHost},
	// HiringThing is white-labelled: one application serves a tenant under the vendor's own
	// domain (<tenant>.hiringthing.com) and under each reseller's, and a slug alone names
	// nothing — nothing in it says which domain answers for it, and the same slug can exist
	// under two resellers. So the board is the whole careers host, the shape hiringthing.yml
	// stores, and every reseller domain needs its own row: an unlisted one is simply
	// unrecognised. These 25 are the domains that file is actually on. ("rippling-ats" is a
	// reseller of THIS platform and is unrelated to ats.rippling.com above, which is Rippling's
	// own ATS on a different host and a different adapter.)
	{"alignhrsolutions", "hiringthing", modeHost},
	{"alphastaff-hiring", "hiringthing", modeHost},
	{"atsmodule", "hiringthing", modeHost},
	{"checkwritersrecruit", "hiringthing", modeHost},
	{"deltahire-ats", "hiringthing", modeHost},
	{"edistoats", "hiringthing", modeHost},
	{"elevate-ats", "hiringthing", modeHost},
	{"esi-hire", "hiringthing", modeHost},
	{"exponentats", "hiringthing", modeHost},
	{"ezhiregov", "hiringthing", modeHost},
	{"gnahiring", "hiringthing", modeHost},
	{"hiringthing", "hiringthing", modeHost},
	{"iconnecthire", "hiringthing", modeHost},
	{"lumberhiring", "hiringthing", modeHost},
	{"nexstarrecruiter", "hiringthing", modeHost},
	{"oasisrecruit", "hiringthing", modeHost},
	{"primepay-recruit", "hiringthing", modeHost},
	{"prismhr-hire", "hiringthing", modeHost},
	{"rippling-ats", "hiringthing", modeHost},
	{"tcr-hire", "hiringthing", modeHost},
	{"teammemberhire-recruit", "hiringthing", modeHost},
	{"topdoghrrecruiting", "hiringthing", modeHost},
	{"topgradinghire", "hiringthing", modeHost},
	{"verahr-hiring", "hiringthing", modeHost},
	{"viewpointhr-ats", "hiringthing", modeHost},

	// --- hostpath: board = "<host>/<site>" (Workday tenant host + first-path-segment site) ---
	{"myworkdayjobs.com", "workday", modeHostPath},

	// --- hosttenantboard: board = "<host>/<tenant>/<guid>" (UKG) ---
	// All four host families, because the board id embeds the host: two US pods, the Canadian
	// data-residency TLD, and the per-tenant rec.pro host. The catalogue holds 2166 boards on
	// *.ultipro.com, 709 on *.rec.pro.ukg.net and 164 on *.ultipro.ca; a rule naming one host
	// leaves the other three unrecognised.
	{"ultipro.com", "ukg", modeHostTenantBoard},
	{"ultipro.ca", "ukg", modeHostTenantBoard},
	{"rec.pro.ukg.net", "ukg", modeHostTenantBoard},

	// --- hostcareers: board = "<host>/<tenant>" (UKG Ready — a DIFFERENT product from ukg above) ---
	// One environment is fronted by several white-label hosts that all serve its tenants, so the
	// host in a pasted link is by construction one that answers for that tenant — which is what
	// makes it safe to keep. It cannot be dropped: a tenant lives in exactly one environment,
	// every other host answers "Company not found" for it, and the environment is not derivable
	// from the tenant id. The regional workforceready hosts are listed per TLD because a host
	// entry keys on a full domain here; an unlisted TLD is unrecognised, never a false board.
	//
	// One board of the 2,230 is stored with a "www." host, which hostname() strips package-wide,
	// so a link to it resolves to the same tenant on the bare host. That board is crawlable too,
	// and boardIdentity folds the pair onto the tenant, so the cost is one duplicate contribution
	// — cheaper than teaching one mode to keep a label the rest of the package normalizes away.
	{"saashr.com", "ukgready", modeHostCareers},
	{"entertimeonline.com", "ukgready", modeHostCareers},
	{"yourpayrollhr.com", "ukgready", modeHostCareers},
	{"mykronos.com", "ukgready", modeHostCareers},
	{"workforceready.com.au", "ukgready", modeHostCareers},
	{"workforceready.eu", "ukgready", modeHostCareers},
}

// queryBoards holds, per matched host entry in modeQuery, the query parameter that names the
// board, the shape its value must have to BE one, and the path the board's listing is served
// from. The canonical collapses to that listing, so a posting URL and the board's own career
// home map to one board — the same collapse the host and pathlocale modes make.
//
// The shape is required, not optional, for the reason pathnumeric exists: a parameter is a much
// weaker signal than a path segment on a board-only host, so without it any value on the host
// reads as a board — and a board that does not exist is the expensive direction, recorded by the
// contribution flow and paid for.
var queryBoards = map[string]struct {
	param, listingPath string
	boardPattern       *regexp.Regexp
}{
	// Paycor Recruiting addresses an employer's portal by a 32-hex clientId: the listing is
	// /career/CareerHome.action?clientId=<board> and a posting is
	// /career/JobIntroduction.action?clientId=<board>&id=<posting>. Nothing but the parameter
	// names the board — the path is the same for every employer on the platform. All 2,442
	// boards in paycor.yml carry that exact shape.
	"recruitingbypaycor.com": {
		param:        "clientId",
		listingPath:  "/career/CareerHome.action",
		boardPattern: regexp.MustCompile(`^[0-9a-f]{32}$`),
	},
}

// apiBoards lists each ATS's OWN API host, where the board sits behind a fixed path prefix
// rather than in the first segment. These are not links a person pastes; they are the XHR a
// career site on the EMPLOYER's own domain makes to load its listing. That request often names
// the board when nothing else on the page does — phantom.com/careers is a plain marketing page
// whose only mention of the Ashby board "phantom" is the posting-api URL.
//
// Matched before atsBoards, which also settles a shadowing bug: boards-api.greenhouse.io is a
// subdomain of greenhouse.io, so the path rule used to read the API version as the board and
// hand back greenhouse/"v1" — a silently wrong board of exactly the kind this package's doc
// comment warns about.
var apiBoards = []struct{ host, source, prefix string }{
	{"api.ashbyhq.com", "ashby", "posting-api/job-board"},
	{"boards-api.greenhouse.io", "greenhouse", "v1/boards"},
	{"api.lever.co", "lever", "v0/postings"},
}

// reservedSegments lists, per matched host entry, the leading path segments that are the
// platform's own machinery and never a tenant. They are skipped in path mode; when nothing
// but reserved segments remains the URL is declined rather than turned into a false board.
//
// Jobvite serves one board both bare (<board>/job/<id>) and behind a portal segment
// (careers/<board>/jobs), so the first segment read "careers" as the employer — the same defect
// SmartRecruiters got pathportal for. Greenhouse's embed URLs carry no board in the path at all
// (the slug is in the `for=` query param, which atsdetect reads), so every one of their path
// words is machinery.
// noBoardFirstSegments lists, per matched host entry, leading path segments that mean the URL
// carries NO board at all — unlike reservedSegments, which are skipped to reach the board behind
// them. Workable's "/j/<id>" is a per-job shortlink: skipping "j" would take the JOB id as the
// company slug, which is worse than declining.
var noBoardFirstSegments = map[string][]string{
	"apply.workable.com": {"j"},
	// Gusto's "/postings/<company-slug>-<job-slug>-<posting-uuid>" names a posting, not a board:
	// the uuid it ends with is the POSTING's, and the board is "<company-slug>-<company-uuid>",
	// which appears nowhere in it. Taking the segment would file a board no crawl can resolve.
	"jobs.gusto.com": {"postings"},
	// Dayforce serves every career site from one host, so its own machinery shares that host with
	// the tenants: "/api/…" is the listing API the site loads over XHR and "/_next/…" is the app's
	// static bundle, which every page on the platform links. Read as a career site they yield the
	// boards "api/geo" and "_next/static" — and boardresolve, which takes the first recognized ATS
	// URL in a fetched page, would meet them before any real one.
	"jobs.dayforcehcm.com": {"api", "_next"},
}

var reservedSegments = map[string][]string{
	"jobs.jobvite.com": {"careers"},
	"greenhouse.io":    {"embed", "job_app", "job_board", "js"},
	// Gusto's board listing is /boards/<board>; "boards" is the platform's word, never a tenant.
	"jobs.gusto.com": {"boards"},
}

// Recognize parses a pasted job link into the company board it belongs to: the source
// (ATS provider), the board slug, and the canonical URL to store. ok=false when the host is
// not a supported ATS or the URL carries no board segment/label.
func Recognize(rawURL string) (source, board, canonical string, ok bool) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", "", "", false
	}
	host := hostname(u)
	if src, board, canonical, ok := recognizeAPI(u, host); ok {
		return src, board, canonical, true
	}
	src, mode, apex, known := matchHost(host)
	if !known {
		return "", "", "", false
	}

	switch mode {
	case modeSubdomain, modeSubdomainChain, modeHost:
		// The vendor's own product host must not be read as a tenant, regardless of which of
		// the three modes matched: all three derive the board from host's leftmost DNS label
		// (bare, chained, or as the whole host), and a platform label like "app" or "help" sits
		// in that exact position for a subdomain tenant just as much as for a bare-host one —
		// e.g. app.recruitee.com and help.bamboohr.com are the vendor's own login/support hosts,
		// not a company named "app" or "help".
		if platformHost(host, apex) {
			return "", "", "", false
		}
		switch mode {
		case modeSubdomain:
			board = subdomainLabel(host, apex)
		case modeSubdomainChain:
			board = subdomainChain(host, apex)
		default:
			board = host // the whole careers host is the tenant identity
		}
		if board == "" {
			return "", "", "", false // bare apex, no tenant label
		}
		// The board IS the host, so the canonical URL is the bare host — collapsing a vacancy
		// URL and the board listing to one board.
		u.RawQuery, u.Fragment, u.Path = "", "", ""
		return src, board, u.String(), true

	case modeHostPath:
		// Workday: board = "<host>/<site>" where site is the first path segment (case-preserved,
		// as the ingest stores it). Workday's public URL may prefix the site with an xx-XX locale
		// the CXS API omits (host/en-US/<site>), so skip a leading locale — a locale-prefixed URL
		// and a bare one resolve to the same board. A URL carrying ONLY a locale (host/en-US, itself
		// a 404 landing) has no derivable site and is left unrecognized rather than taken as a false
		// "en-US" board. Canonical strips to scheme://host/site.
		site := firstSegmentAfterLocale(u)
		// A Workday per-job URL can reach us with no site at all (host/job/... or
		// host/details/...), and those segments are the POSTING, never a career site. Taking one
		// as the site yields "<host>/job" — a board that does not exist but looks new, so the
		// contribution flow records and pays for it.
		if site == "" || site == "job" || site == "details" {
			return "", "", "", false // bare host, locale-only, or a per-job path with no site
		}
		u.RawQuery, u.Fragment = "", ""
		u.Path = "/" + site
		return src, host + "/" + site, u.String(), true

	case modeHostTenantBoard:
		// UKG: <host>/<tenant>/JobBoard/<guid>/… → board "<host>/<tenant>/<guid>". The literal
		// "JobBoard" segment between the two is what proves the URL names a board at all; without
		// it the path is some other part of the tenant's site (a login, an OpportunityDetail
		// shortlink) and there is no guid to take. Declining is the only safe reading — the
		// tenant alone is a board id the adapter rejects, and a rejected board still costs a
		// contribution row and a crawl slot.
		tenant, guid, ok := ukgTenantBoard(u)
		if !ok {
			return "", "", "", false
		}
		// Lower-cased: UKG answers either spelling (verified live against a real tenant, upper and
		// lower, both 200) and every board in the catalogue is stored lower-case, so keeping a
		// link's own casing would file a board we already crawl as a new one. This is the opposite
		// of Workday above, whose site segment is a human-chosen name the ingest stores verbatim.
		board = strings.ToLower(host + "/" + tenant + "/" + guid)
		u.RawQuery, u.Fragment = "", ""
		u.Path = "/" + tenant + "/JobBoard/" + guid
		return src, board, u.String(), true

	case modeHostCareers:
		// UKG Ready: <host>/ta/<tenant>.careers[?ShowJob=…] → board "<host>/<tenant>". Lower-cased
		// because tenant ids are case-insensitive at the platform and ukgready.yml holds them
		// lower-case, so keeping a link's own spelling would file a board we already crawl as new.
		tenant, ok := ukgReadyTenant(u)
		if !ok {
			return "", "", "", false
		}
		u.RawQuery, u.Fragment = "", ""
		u.Path = "/ta/" + tenant + ".careers"
		return src, host + "/" + tenant, u.String(), true

	case modeQuery:
		// Paycor: the board is a query parameter, and the canonical is the board's own listing —
		// so a posting and the career home collapse to one. Lower-cased for the same reason as
		// UKG Ready: the platform serves either spelling of the hex client id and paycor.yml
		// holds it lower-case.
		q, configured := queryBoards[apex]
		if !configured || q.boardPattern == nil {
			return "", "", "", false
		}
		// The pattern rejects an absent or empty parameter along with a malformed one, so a link
		// on the host that names no board is declined rather than turned into one.
		if board = strings.ToLower(u.Query().Get(q.param)); !q.boardPattern.MatchString(board) {
			return "", "", "", false
		}
		u.Fragment = ""
		u.Path = q.listingPath
		u.RawQuery = url.Values{q.param: {board}}.Encode()
		return src, board, u.String(), true

	case modePathLocalePair:
		// Dayforce: <host>/<culture?>/<tenant>/<site>/… → board "<tenant>/<site>". The culture is
		// dropped, not kept: it selects which translations of a site's postings to read and a
		// posting keeps one id across them, which is why the ingest adapter folds it off too. The
		// canonical collapses to the culture-free site root (verified live: it answers 200 with and
		// without one). Lower-cased — tenant and site are case-insensitive at the platform and
		// dayforce.yml holds them lower-case.
		segs := segmentsAfterLocale(u)
		// The no-board check reads the segment the board would START at, not the URL's first —
		// the platform's own paths take a locale prefix too, and "/en-US/api/geo/…" would
		// otherwise walk past the guard and name the board "api/geo".
		if len(segs) < 2 || segs[0] == "" || segs[1] == "" || slices.Contains(noBoardFirstSegments[apex], segs[0]) {
			return "", "", "", false // bare host, locale-only, a tenant with no site, or machinery
		}
		board = strings.ToLower(segs[0] + "/" + segs[1])
		u.RawQuery, u.Fragment = "", ""
		u.Path = "/" + board
		return src, board, u.String(), true

	case modePathPortal:
		// SmartRecruiters' Apply button leads to a one-click form under the product's own
		// path, which names the employer instead of leading with it. It is matched first
		// because the generic rule below would read "oneclick-ui" as the board — a tenant
		// that does not exist, which the contribution flow would record and pay for.
		if m := smartrecruitersOneClick.FindStringSubmatch(u.Path); m != nil {
			u.RawQuery, u.Fragment = "", ""
			u.Path = "/" + m[1]
			return src, m[1], u.String(), true
		}
		// Otherwise the employer is the segment immediately before the posting, which is
		// the first segment on a bare URL and the second behind a portal segment. Only a
		// recognizable posting segment shifts the board along; any other shape falls through to
		// the first segment, so an unknown URL shape can't invent a board.
		board = segmentBeforePosting(u, smartrecruitersPosting)
		if board == "" {
			return "", "", "", false
		}
		u.RawQuery, u.Fragment = "", ""
		return src, board, u.String(), true

	case modePathLocale:
		// Rippling: skip a leading xx-XX locale segment (ats.rippling.com/en-GB/<board>/…),
		// which the board API omits, and collapse the canonical to the board root so a
		// locale-prefixed vacancy, a bare vacancy, and the listing all map to one board.
		board = firstSegmentAfterLocale(u)
		if board == "" {
			return "", "", "", false
		}
		u.RawQuery, u.Fragment = "", ""
		u.Path = "/" + board
		return src, board, u.String(), true
	}

	// modePath / modePathNumeric: the board is the first path segment that isn't platform
	// machinery — declined outright when the path leads with a segment that carries no board.
	if leadsWithNoBoard(u, noBoardFirstSegments[apex]) {
		return "", "", "", false
	}
	board = firstTenantSegment(u, reservedSegments[apex])
	if board == "" {
		return "", "", "", false
	}
	if mode == modePathNumeric && !allDigits(board) {
		return "", "", "", false
	}
	u.RawQuery = ""
	u.Fragment = ""
	u.Path = strings.TrimSuffix(strings.TrimSuffix(u.Path, "/"), "/apply")
	return src, board, u.String(), true
}

// recognizeAPI resolves a URL on an ATS's own API host, where the board is the segment right
// after the entry's fixed path prefix. The canonical collapses to that prefix + board, so every
// query-parametered variant of the same API call maps to one board.
func recognizeAPI(u *url.URL, host string) (source, board, canonical string, ok bool) {
	for _, a := range apiBoards {
		if host != a.host {
			continue
		}
		rest, found := strings.CutPrefix(strings.Trim(u.Path, "/"), a.prefix+"/")
		if !found {
			return "", "", "", false
		}
		if board, _, _ = strings.Cut(rest, "/"); board == "" {
			return "", "", "", false
		}
		u.RawQuery, u.Fragment = "", ""
		u.Path = "/" + a.prefix + "/" + board
		return a.source, board, u.String(), true
	}
	return "", "", "", false
}

// matchHost returns the ATS entry for a host. path/subdomain entries match the host exactly or
// as a subdomain of the entry host (the returned apex). A host entry keys on a domain LABEL
// (e.g. "zohorecruit") and matches any host containing ".<label>." — a tenant subdomain on any
// regional TLD (<tenant>.zohorecruit.eu/.com/.in); the bare apex ("zohorecruit.com") does not
// match, so it is never taken as a board.
func matchHost(host string) (source, mode, apex string, ok bool) {
	for _, a := range atsBoards {
		if a.mode == modeHost {
			if strings.Contains(host, "."+a.host+".") {
				return a.source, a.mode, a.host, true
			}
			continue
		}
		if host == a.host || strings.HasSuffix(host, "."+a.host) {
			return a.source, a.mode, a.host, true
		}
	}
	return "", "", "", false
}

// subdomainChain returns every DNS label of host under apex:
// "thefjx.global.huntflow.io","huntflow.io" → "thefjx.global"; "huntflow.io",… → "" (no tenant).
func subdomainChain(host, apex string) string {
	sub := strings.TrimSuffix(host, "."+apex)
	if sub == host {
		return ""
	}
	return sub
}

// subdomainLabel returns the leftmost DNS label of host under apex:
// "acme.recruitee.com","recruitee.com" → "acme"; "recruitee.com",… → "" (no tenant).
//
// A remainder with more than one label yields NO board, deliberately. These adapters crawl
// "<board>.<apex>", so a regional or nested host like "uk-ext.eu.csod.com" has no such form —
// taking "uk-ext" names a host that does not exist. That is the expensive direction: the
// contribution flow pays for a board it believes is new. Where a platform genuinely nests a
// tenant under an instance, the entry uses subdomainchain and keeps every label.
func subdomainLabel(host, apex string) string {
	sub := subdomainChain(host, apex)
	if strings.Contains(sub, ".") {
		return ""
	}
	return sub
}

// platformLabels are the leftmost DNS labels a multi-tenant ATS uses for its own product hosts
// rather than for a tenant. In host mode the whole host is the board, so without this a link to
// the platform's own app (which every tenant career site carries) reads as a tenant — and
// boardresolve, which accepts the first recognized ATS URL found in a page, then records that
// as the employer's board.
var platformLabels = map[string]bool{
	"app": true, "dashboard": true, "admin": true, "api": true,
	"support": true, "help": true, "blog": true, "docs": true,
	// tt.teamtailor.com is Teamtailor's own "powered by" tracking/short-link host, linked from
	// every tenant career site's footer — not a tenant.
	"tt": true,
}

// platformLabelsByApex lists the labels that are ONE platform's own hosts — the per-platform
// companion to platformLabels, for a label another platform legitimately lets a tenant have.
// "apply" is both: HRM Direct serves every tenant's application form from apply.hrmdirect.com,
// which a bare subdomain rule reads as a company called "apply", while apply.recruitee.com is a
// board recruitee.yml actually tracks. Declining it everywhere would drop a board we crawl.
var platformLabelsByApex = map[string][]string{
	"hrmdirect.com": {"apply"},
}

// platformHost reports whether host is the ATS's own product host rather than a tenant's —
// either a label no multi-tenant ATS gives a tenant, or one this platform in particular keeps.
func platformHost(host, apex string) bool {
	label, _, ok := strings.Cut(host, ".")
	if !ok {
		return false
	}
	return platformLabels[label] || slices.Contains(platformLabelsByApex[apex], label)
}

// smartrecruitersPosting matches a SmartRecruiters posting segment: the posting id (numeric or a
// UUID) followed by the title slug.
var smartrecruitersPosting = regexp.MustCompile(`^(?:[0-9]{6,}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})-`)

// smartrecruitersOneClick matches the one-click apply form the Apply button leads to, whose
// path names the employer rather than starting with it:
// /oneclick-ui/company/<board>/publication/<uuid>.
var smartrecruitersOneClick = regexp.MustCompile(`^/oneclick-ui/company/([^/]+)/publication/[^/]+/?$`)

// segmentBeforePosting returns the path segment immediately before the last segment when that
// last segment is a posting (per isPosting), and the first segment otherwise — a board listing
// URL carries no posting, and an unfamiliar shape must not shift the board off the first segment.
func segmentBeforePosting(u *url.URL, isPosting *regexp.Regexp) string {
	p := strings.Trim(u.Path, "/")
	if p == "" {
		return ""
	}
	segs := strings.Split(p, "/")
	if n := len(segs); n >= 2 && isPosting.MatchString(segs[n-1]) {
		return segs[n-2]
	}
	return segs[0]
}

// localeSegment matches an xx-XX language-COUNTRY locale (e.g. en-GB) — the optional leading
// path segment Rippling's public board site inserts before the tenant. Tenant slugs are
// lowercase (satomic, 360-fire-flood), so the uppercase country code never collides.
// The country half is matched case-insensitively: the canonical spelling is xx-XX, but the
// lowercase form appears in real Workday links, and reading "en-us" as a career site produces
// the board "<host>/en-us" — a board that does not exist, which the contribution flow would
// record as new and pay for.
var localeSegment = regexp.MustCompile(`^[a-z]{2}-[A-Za-z]{2}$`)

// ukgTenantBoard pulls the tenant and board guid out of a UKG path,
// "<tenant>/JobBoard/<guid>[/…]". ok is false unless all three parts are present and in that
// order: the "JobBoard" marker is the only thing that distinguishes a board URL from the rest
// of a tenant's site, and both ids must be non-empty.
func ukgTenantBoard(u *url.URL) (tenant, guid string, ok bool) {
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segs) < 3 || !strings.EqualFold(segs[1], "JobBoard") {
		return "", "", false
	}
	if segs[0] == "" || segs[2] == "" {
		return "", "", false
	}
	return segs[0], segs[2], true
}

// firstSegmentAfterLocale returns the first path segment that isn't a leading xx-XX locale — the
// tenant board in ats.rippling.com/<locale?>/<board>/… (Rippling) or the site in a Workday
// host/<locale?>/<site> URL. "" when the path is empty or carries only a locale.
func firstSegmentAfterLocale(u *url.URL) string {
	if segs := segmentsAfterLocale(u); len(segs) > 0 {
		return segs[0]
	}
	return ""
}

// segmentsAfterLocale returns u's path segments with a leading xx-XX locale dropped, nil for an
// empty path. Rippling and Workday need only the first of them; Dayforce's board is the first two.
func segmentsAfterLocale(u *url.URL) []string {
	p := strings.Trim(u.Path, "/")
	if p == "" {
		return nil
	}
	segs := strings.Split(p, "/")
	if localeSegment.MatchString(segs[0]) {
		segs = segs[1:]
	}
	return segs
}

// ukgReadyTenant returns the lower-cased tenant a UKG Ready career-page path addresses,
// "/ta/<tenant>.careers[/…]". ok is false unless the path leads with the platform's "ta" segment
// AND the one after it carries the ".careers" suffix: that suffix is what proves the segment
// names a career site at all. Everything else under /ta is the tenant's own application — the
// SPA's REST API lives at /ta/rest/… — and carries no board.
func ukgReadyTenant(u *url.URL) (string, bool) {
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segs) < 2 || !strings.EqualFold(segs[0], "ta") {
		return "", false
	}
	tenant, found := strings.CutSuffix(strings.ToLower(segs[1]), ".careers")
	if !found || tenant == "" {
		return "", false
	}
	return tenant, true
}

// hostname is u's lowercased hostname with a leading "www." stripped.
func hostname(u *url.URL) string {
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

// firstTenantSegment returns u's first path segment that is not one of reserved — the platform
// path words that are never a tenant on this host. "/acme/jobs/1" → "acme"; "/careers/ness/jobs"
// with reserved{"careers"} → "ness"; "/" or a path of nothing but reserved words → "".
func firstTenantSegment(u *url.URL, reserved []string) string {
	p := strings.Trim(u.Path, "/")
	if p == "" {
		return ""
	}
	for _, seg := range strings.Split(p, "/") {
		if !slices.Contains(reserved, seg) {
			return seg
		}
	}
	return ""
}

// leadsWithNoBoard reports whether u's first path segment marks a URL shape that carries no
// board (see noBoardFirstSegments).
func leadsWithNoBoard(u *url.URL, none []string) bool {
	if len(none) == 0 {
		return false
	}
	p := strings.Trim(u.Path, "/")
	if p == "" {
		return false
	}
	first, _, _ := strings.Cut(p, "/")
	return slices.Contains(none, first)
}

// allDigits reports whether s is non-empty and only ASCII digits.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
