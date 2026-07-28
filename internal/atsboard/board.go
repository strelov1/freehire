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
package atsboard

import (
	"net/url"
	"regexp"
	"strings"
)

// Board extraction modes, each matching how the ingest adapter namespaces jobs.external_id:
//   - path:      board = the first path segment on a fixed host (jobs.lever.co/<board>/…).
//   - pathlocale: like path, but a leading xx-XX locale segment is skipped first — Rippling's
//     public site prefixes the board with a locale (ats.rippling.com/en-GB/<board>/…) that its
//     board API omits, so both URL shapes must resolve to the same board.
//   - subdomain: board = the leftmost DNS label under a fixed apex (<board>.recruitee.com).
//   - host:      board = the whole careers host (the tenant identity IS the host, and the TLD
//     varies by region, e.g. <tenant>.zohorecruit.eu / .com / .in).
//   - hostpath:  board = "<host>/<first path segment>" (Workday: the tenant is the host, the
//     site is the first path segment, e.g. acme.wd1.myworkdayjobs.com/Careers).
//
// For subdomain and host the board IS the host; for hostpath it is host + site. In all these the
// canonical URL is stripped to that board, collapsing a vacancy URL and the board listing to one.
const (
	modePath       = "path"
	modePathLocale = "pathlocale"
	modeSubdomain  = "subdomain"
	modeHost       = "host"
	modeHostPath   = "hostpath"
)

// atsBoards lists the supported multi-tenant ATS: a host (exact or subdomain-suffix match) →
// its source key and extraction mode. Hosts were verified against each adapter's public job
// URL. A wrong/missing entry is fail-safe: the link simply isn't recognized, never a
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
	{"careers.pageuppeople.com", "pageup", modePath},
	{"oportunidades.mindsight.com.br", "mindsight", modePath},
	{"careers.hireology.com", "hireology", modePath},
	{"jobs.smartrecruiters.com", "smartrecruiters", modePath},
	{"careers.smartrecruiters.com", "smartrecruiters", modePath},
	{"recruiting.ultipro.com", "ukg", modePath},

	// --- pathlocale: like path, skipping a leading xx-XX locale segment ---
	{"ats.rippling.com", "rippling", modePathLocale},

	// --- subdomain: board = leftmost DNS label under the apex ---
	{"recruitee.com", "recruitee", modeSubdomain},
	{"bamboohr.com", "bamboohr", modeSubdomain},
	{"breezy.hr", "breezy", modeSubdomain},
	{"freshteam.com", "freshteam", modeSubdomain},
	{"huntflow.io", "huntflow", modeSubdomain},
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
	{"careers-page.com", "careerspage", modeSubdomain},
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

	// --- host: board = the whole careers host (regional TLD varies) ---
	{"zohorecruit", "zohorecruit", modeHost},
	{"teamtailor", "teamtailor", modeHost}, // <tenant>.teamtailor.com; custom-domain career sites are absent (not URL-derivable)
	{"factorial", "factorial", modeHost},   // <tenant>.factorial.<tld>
	{"factorialhr", "factorial", modeHost}, // the .com.br/.pt/… base-domain variant — ONE ingest adapter serves both, and it reports "factorial"

	// --- hostpath: board = "<host>/<site>" (Workday tenant host + first-path-segment site) ---
	{"myworkdayjobs.com", "workday", modeHostPath},
}

// Recognize parses a job link into the company board it belongs to: the source (ATS
// provider), the board slug, and the canonical URL to store. ok=false when the host is
// not a supported ATS or the URL carries no board segment/label.
func Recognize(rawURL string) (source, board, canonical string, ok bool) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", "", "", false
	}
	host := hostname(u)
	src, mode, apex, known := matchHost(host)
	if !known {
		return "", "", "", false
	}

	switch mode {
	case modeSubdomain, modeHost:
		if mode == modeSubdomain {
			board = subdomainLabel(host, apex)
		} else {
			board = host // the whole careers host is the tenant identity
		}
		if board == "" || reservedLabels[leftmostLabel(host)] {
			// Either a bare apex with no tenant label, or one of the platform's OWN hosts.
			// The second case is not hypothetical: every Teamtailor career site links to
			// app.teamtailor.com, and in host mode the whole host is the board — so the
			// platform's console was once recorded as an employer's board.
			return "", "", "", false
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
		if site == "" {
			return "", "", "", false // bare host or locale-only, no site
		}
		u.RawQuery, u.Fragment = "", ""
		u.Path = "/" + site
		return src, host + "/" + site, u.String(), true

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

	// modePath: the board is the first path segment.
	board = firstPathSegment(u)
	if board == "" {
		return "", "", "", false
	}
	u.RawQuery = ""
	u.Fragment = ""
	u.Path = strings.TrimSuffix(strings.TrimSuffix(u.Path, "/"), "/apply")
	return src, board, u.String(), true
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

// reservedLabels are subdomains an ATS serves ITSELF under — its console, marketing site, or
// API — never a customer tenant. They matter because in host and subdomain mode the host IS
// the board, so without this an intake pointing at the vendor's own console records the
// vendor as an employer. Erring towards declining is safe: a declined link is merely
// unrecognised, whereas a false board pollutes the catalogue under a name nobody can crawl.
var reservedLabels = map[string]bool{
	"app": true, "api": true, "www": true, "admin": true, "portal": true,
	"help": true, "support": true, "docs": true, "blog": true, "status": true,
	"login": true, "auth": true, "account": true, "accounts": true,
	"static": true, "cdn": true, "assets": true, "media": true, "img": true,
	"mail": true, "email": true, "go": true, "partners": true, "developers": true,
}

// leftmostLabel returns the first DNS label of a host ("app.teamtailor.com" → "app").
func leftmostLabel(host string) string {
	if i := strings.IndexByte(host, '.'); i >= 0 {
		return host[:i]
	}
	return host
}

// subdomainLabel returns the leftmost DNS label of host under apex:
// "acme.recruitee.com","recruitee.com" → "acme"; "recruitee.com",… → "" (no tenant).
func subdomainLabel(host, apex string) string {
	sub := strings.TrimSuffix(host, "."+apex)
	if sub == host || sub == "" {
		return ""
	}
	if i := strings.IndexByte(sub, '.'); i >= 0 {
		return sub[:i]
	}
	return sub
}

// localeSegment matches an xx-XX language-COUNTRY locale (e.g. en-GB) — the optional leading
// path segment Rippling's public board site inserts before the tenant. Tenant slugs are
// lowercase (satomic, 360-fire-flood), so the uppercase country code never collides.
var localeSegment = regexp.MustCompile(`^[a-z]{2}-[A-Z]{2}$`)

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

// firstPathSegment returns u's first non-empty path segment ("/acme/jobs/1" → "acme",
// "/acme" → "acme", "/" → "").
func firstPathSegment(u *url.URL) string {
	p := strings.Trim(u.Path, "/")
	if p == "" {
		return ""
	}
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return p
}
