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
	{"recruiting.ultipro.com", "ukg", modePath},
	// Manatal's hosted career-page domain is PATH-based (careers-page.com/<tenant>/job/<id>),
	// not a tenant subdomain, and its source is manatal: careerspage.yml is deliberately empty
	// because every careers-page.com tenant is served by the Manatal adapter.
	{"careers-page.com", "manatal", modePath},

	// --- pathportal: board = the segment before the posting segment ---
	{"jobs.smartrecruiters.com", "smartrecruiters", modePathPortal},
	{"careers.smartrecruiters.com", "smartrecruiters", modePathPortal},

	// --- pathlocale: like path, skipping a leading xx-XX locale segment ---
	{"ats.rippling.com", "rippling", modePathLocale},

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
	{"catsone.com", "catsone", modeSubdomain},
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
	{"careers.hibob.com", "hibob", modeSubdomain}, // HiBob's careers module: <tenant>.careers.hibob.com

	// --- subdomainchain: board = every label under the apex (tenant nested under a region) ---
	{"huntflow.io", "huntflow", modeSubdomainChain},

	// --- host: board = the whole careers host (regional TLD varies) ---
	{"zohorecruit", "zohorecruit", modeHost},
	{"teamtailor", "teamtailor", modeHost}, // <tenant>.teamtailor.com; custom-domain career sites are absent (not URL-derivable)
	{"factorial", "factorial", modeHost},   // <tenant>.factorial.<tld>
	{"factorialhr", "factorial", modeHost}, // the .com.br/.pt/… base-domain variant — ONE ingest adapter serves both, and it reports "factorial"

	// --- hostpath: board = "<host>/<site>" (Workday tenant host + first-path-segment site) ---
	{"myworkdayjobs.com", "workday", modeHostPath},
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
}

var reservedSegments = map[string][]string{
	"jobs.jobvite.com": {"careers"},
	"greenhouse.io":    {"embed", "job_app", "job_board", "js"},
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
		if platformHost(host) {
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

	case modePathPortal:
		// SmartRecruiters: the employer is the segment immediately before the posting, which is
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

// platformHost reports whether host is the ATS's own product host rather than a tenant's.
func platformHost(host string) bool {
	label, _, ok := strings.Cut(host, ".")
	return ok && platformLabels[label]
}

// smartrecruitersPosting matches a SmartRecruiters posting segment: the posting id (numeric or a
// UUID) followed by the title slug.
var smartrecruitersPosting = regexp.MustCompile(`^(?:[0-9]{6,}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})-`)

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

// firstSegmentAfterLocale returns the first path segment that isn't a leading xx-XX locale — the
// tenant board in ats.rippling.com/<locale?>/<board>/… (Rippling) or the site in a Workday
// host/<locale?>/<site> URL. "" when the path is empty or carries only a locale.
func firstSegmentAfterLocale(u *url.URL) string {
	p := strings.Trim(u.Path, "/")
	if p == "" {
		return ""
	}
	segs := strings.Split(p, "/")
	if localeSegment.MatchString(segs[0]) {
		segs = segs[1:]
	}
	if len(segs) == 0 {
		return ""
	}
	return segs[0]
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
