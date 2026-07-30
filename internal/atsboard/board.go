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
	{"careers.pageuppeople.com", "pageup", modePath},
	{"oportunidades.mindsight.com.br", "mindsight", modePath},
	{"careers.hireology.com", "hireology", modePath},
	{"recruiting.ultipro.com", "ukg", modePath},

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

// Recognize parses a pasted job link into the company board it belongs to: the source
// (ATS provider), the board slug, and the canonical URL to store. ok=false when the host is
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
	case modeSubdomain, modeSubdomainChain, modeHost:
		if mode == modeSubdomain {
			board = subdomainLabel(host, apex)
		} else if mode == modeSubdomainChain {
			board = subdomainChain(host, apex)
		} else if !platformHost(host) {
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
		if site == "" {
			return "", "", "", false // bare host or locale-only, no site
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
func subdomainLabel(host, apex string) string {
	sub := subdomainChain(host, apex)
	if i := strings.IndexByte(sub, '.'); i >= 0 {
		return sub[:i]
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
