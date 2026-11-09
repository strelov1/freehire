package atsboard

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	comeetPattern = pathPattern{
		path:    regexp.MustCompile(`^/jobs/([A-Za-z0-9_.-]+/[0-9A-Za-z]{2}\.[0-9A-Za-z]{3})(?:/|$)`),
		listing: "/jobs/%s",
	}
	neogovPattern = pathPattern{
		path:      regexp.MustCompile(`^/careers/([A-Za-z0-9_-]+)(?:/|$)`),
		reject:    regexp.MustCompile(`^(?i:home)$`),
		listing:   "/careers/%s",
		hostBoard: true,
	}
)

// tenantHostPostingSegments names, per modeTenantHost apex, a path segment a URL must carry to be
// read as a tenant's. Phenom serves its CDN and assets from subdomains of the same domains its
// tenants live on (cdn.phenompeople.com is on every Phenom career page, custom-domain ones
// included), and boardresolve feeds every URL a careers page carries through Recognize — so
// without this, any Phenom-hosted page would resolve to the vendor's CDN as a board. A posting
// URL always has a "job" segment (/us/en/job/<id>/<slug>, /job/<id>); a career-site home does
// not, and is left unrecognised rather than guessed.
var tenantHostPostingSegments = map[string]string{
	"phenompro.com":    "job",
	"phenompeople.net": "job",
	"phenompeople.com": "job",
	// Every Darwinbox portal URL carries it (/ms/candidatev2/main/careers/jobDetails/<id>,
	// /ms/candidate/careers/<id>); the tenant's HRMS pages on the same host do not.
	"darwinbox.in":  "careers",
	"darwinbox.com": "careers",
	// Every greytHR Hire URL sits under /hire/ (/hire/jobs/, /hire/jobs/<slug>); the HRMS does not.
	"greythr.com": "hire",
}

// keyedSegmentBoards holds, per modeKeyedSegment host entry, the key segment the board follows
// and the shape the board must have.
var keyedSegmentBoards = map[string]struct {
	key          string
	boardPattern *regexp.Regexp
}{
	// Workstream's employer id: the eight lower-case hex characters every canonical career-site
	// URL opens with (internal/ingest/sources/workstream.go).
	"workstream.us": {key: "j", boardPattern: regexp.MustCompile(`^[0-9a-f]{8}$`)},
	// LiveHire's tenant segment: the careers path opens with /careers/<segment>, and the segment
	// is lower-case alphanumeric with hyphens (e.g. "flybuys"). The /jobs and /job/… subpages
	// under /careers are covered by the same pattern — modeKeyedSegment matches the segment
	// right after the key wherever it sits, and the canonical collapses to the board listing.
	"livehire.com": {key: "careers", boardPattern: regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)},
}

// businessID is the shape of a decoded modeEncodedCareers board: a decimal id with no leading zero.
var businessID = regexp.MustCompile(`^[1-9][0-9]*$`)

// queryBoards holds, per matched host entry in modeQuery, the query parameter that names the
// board, the shape its value must have to BE one, and the path the board's listing is served
// from. The canonical collapses to that listing, so a posting URL and the board's own career
// home map to one board — the same collapse the host and pathlocale modes make.
//
// The shape is required, not optional, for the reason pathnumeric exists: a parameter is a much
// weaker signal than a path segment on a board-only host, so without it any value on the host
// reads as a board — and a board that does not exist is the expensive direction, recorded by the
// contribution flow and paid for.
//
// The value is folded to lower case unless keepCase is set. tenantHost makes the board
// "<host>/<value>" and admits only a host exactly one tenant label under the entry's domain, for
// a platform that addresses a tenant by its careers host AND a parameter. listingQuery holds any
// fixed parameters the listing URL needs beyond the one that names the board.
var queryBoards = map[string]struct {
	param, listingPath string
	boardPattern       *regexp.Regexp
	keepCase           bool
	tenantHost         bool
	listingQuery       url.Values
	// altParam names a second parameter that carries the board inside a longer value, for a
	// platform whose posting links name the posting rather than the board; altPattern's first
	// capture group is the board. It is read only when param is absent or not a board.
	altParam   string
	altPattern *regexp.Regexp
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
	// HR-Manager addresses a customer by its ALIAS, and only the customer-level pages carry it:
	// /vacancies/list.aspx?customer=<alias> (the listing) and the job-agent signup
	// /Agent/Subscription.aspx?customer=<alias>. A posting (/ApplicationInit.aspx?cid=316&…)
	// names only the numeric cid, which the job-portal API the adapter reads does not accept,
	// so it carries no `customer` and is declined. An alias always contains a letter;
	// `customer=316` answers the platform's "unknown department" page.
	"candidate.hr-manager.net": {
		param:        "customer",
		listingPath:  "/vacancies/list.aspx",
		boardPattern: regexp.MustCompile(`^[a-z0-9_-]*[a-z][a-z0-9_-]*$`),
	},
	// Tenstreet's Pulse pages name a carrier by its numeric company_id on every page: the
	// carrier's profile /share_app_index.php?company_id=<board> (the listing the adapter reads)
	// and a posting /share_app_show_active_jobs.php?active_job_id=<id>&company_id=<board>&….
	// Its other apply host, intelliapp.driverapponline.com/c/<slug>, names no company id and
	// is not recognised.
	"pulse.tenstreet.com": {
		param:        "company_id",
		listingPath:  "/share_app_index.php",
		boardPattern: regexp.MustCompile(`^[0-9]+$`),
	},
	// eRecruiter names a company's board by its 32-hex cfg token on every page of the career
	// widget — a posting is /Offer.aspx?oid=<id>&cfg=<board>&… (internal/ingest/sources/erecruiter.go).
	// The host has no human listing page (the widget is embedded on the employer's own site), so
	// the canonical is the listing the adapter reads. The token is kept as spelled: the platform
	// serves either case and the catalogue stores the spelling of the embed it was harvested from.
	"skk.erecruiter.pl": {
		param:        "cfg",
		listingPath:  "/GetHtml.ashx",
		boardPattern: regexp.MustCompile(`^[0-9A-Fa-f]{32}$`),
		keepCase:     true,
		listingQuery: url.Values{"grid": {"rows"}, "pn": {"1"}},
	},
	// Eightfold's board is "<careers host>/<tenant domain>" (internal/ingest/sources/eightfold.go):
	// the tenant key is the `domain` parameter every career-site page carries
	// (<tenant>.eightfold.ai/careers?domain=eaton.com). It is not derivable from the host — a
	// link without it is declined. `microsite`, which some posting links carry instead, names a
	// microsite of the tenant rather than the tenant itself and is not read.
	"eightfold.ai": {
		param:        "domain",
		listingPath:  "/careers",
		boardPattern: regexp.MustCompile(`^[a-z0-9-]+(?:\.[a-z0-9-]+)+$`),
		tenantHost:   true,
	},
	// CVWarehouse names a customer's career site by its companyGuid on every page: the listing is
	// /?companyGuid=<board> and a vacancy /?companyGuid=<board>&job=<id>
	// (internal/ingest/sources/cvwarehouse.go). The host serves the platform's own pages too, so a
	// link without the parameter is declined.
	"jobpage.cvwarehouse.com": {
		param:        "companyGuid",
		listingPath:  "/",
		boardPattern: regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`),
	},
	// helixjobs.com, the portal Perbility runs for its concludis employers, names an employer
	// by its numeric instance id: the listing is /_/joblist?iid=<board>, while a posting
	// (/_/jobad?prj=<board>P<position>) and its apply form (/_/apply/form?prj=…) name only the
	// posting, whose prefix is the iid (internal/ingest/sources/helixjobs.go). The portal serves
	// both spellings of the separator. An employer's vanity label (apetito-ag.helixjobs.com)
	// names the same pages, so the host is not the board.
	"helixjobs.com": {
		param:        "iid",
		listingPath:  "/_/joblist",
		boardPattern: regexp.MustCompile(`^[1-9][0-9]*$`),
		altParam:     "prj",
		altPattern:   regexp.MustCompile(`^([1-9][0-9]*)p[0-9]+$`),
	},
}

// queryPairBoards holds, per matched host entry in modeQueryPair, the two query parameters that
// together name the board, the shape each value must have, and the path the board's listing is
// served from. It is queryBoards one parameter wider, and exists for the same reason: on a host
// that serves every tenant from one path, only the parameters tell them apart.
//
// The board is written "<first>:<second>" because that is how the ingest adapters take it, not
// because the URL reads that way — sources/adp.go cuts the board on the colon into the cid and
// ccId its API needs, and the catalogue's ~2,800 adp boards are all stored in that form.
//
// foldFirst puts the first value in the case the catalogue stores, since both platforms serve
// either spelling and a link's own casing would otherwise file a crawled board as new. The
// second value is a number on both and has no case to fold. listingQuery holds any fixed
// parameters the listing URL needs beyond the two that name the board.
var queryPairBoards = map[string]struct {
	first, second, listingPath string
	firstPattern               *regexp.Regexp
	secondPattern              *regexp.Regexp
	foldFirst                  func(string) string
	listingQuery               url.Values
}{
	// ADP Workforce Now: the recruitment page is
	// /mascsr/default/mdf/recruitment/recruitment.html?cid=<uuid>&ccId=<id>&jobId=<posting>.
	// cid is the tenant, ccId the career centre within it — one tenant runs several, and the
	// adapter's listing call sends both. The ccId is a digits-and-underscore id ("19000101_000001",
	// "9201289910657_2"), never a uuid, so the two cannot be confused for one another.
	//
	// myjobs.adp.com is a DIFFERENT ADP product (a hosted career site addressed by a name slug)
	// with its own adapter and its own row above; reading its slug as an adp board would produce
	// exactly the silently-wrong board this package exists to avoid.
	"workforcenow.adp.com": {
		first:         "cid",
		second:        "ccId",
		listingPath:   "/mascsr/default/mdf/recruitment/recruitment.html",
		firstPattern:  regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`),
		secondPattern: regexp.MustCompile(`^[0-9][0-9_]*$`),
		foldFirst:     strings.ToLower,
	},
	// Taleo Business Edition: every URL on <pod>.tbe.taleo.net names the employer by its org code
	// and one of its career sites by number — requisition.jsp?org=<ORG>&cws=<n>&rid=<posting>,
	// v2/applyRequisition?…, searchResults?… — and sources/taleobe.go takes exactly that pair.
	// The pod in the path is left out of the board: the dispatcher resolves an org to its pod
	// from any TBE host, and orgs move between pods. The canonical is therefore the dispatcher's
	// career-site redirect on the link's own host, which lands on the listing wherever the org
	// lives now. A link with an empty cws (the platform renders some that way) is declined.
	"tbe.taleo.net": {
		first:         "org",
		second:        "cws",
		listingPath:   "/dispatcher/servlet/DispatcherServlet",
		firstPattern:  regexp.MustCompile(`^[A-Z0-9_]+$`),
		secondPattern: regexp.MustCompile(`^[0-9]+$`),
		foldFirst:     strings.ToUpper,
		listingQuery:  url.Values{"act": {"redirectCws"}},
	},
}
