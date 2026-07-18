package contribution

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
// URL. A wrong/missing entry is fail-safe: the link simply isn't recognized (422), never a
// false board. Single-company brands, aggregators, and custom-domain ATS (Taleo, SuccessFactors,
// Oracle, and Workday tenants on their own domain) are absent — their board can't be derived
// from a URL. Workday's standard *.myworkdayjobs.com hosts ARE derivable (host + site).
var atsBoards = []struct{ host, source, mode string }{
	// --- path: board = first path segment on a fixed host ---
	{"greenhouse.io", "greenhouse", modePath},
	{"jobs.lever.co", "lever", modePath},
	{"jobs.ashbyhq.com", "ashby", modePath},
	{"apply.workable.com", "workable", modePath},
	{"jobs.deel.com", "deel", modePath},
	{"jobs.gem.com", "gem", modePath},
	{"jobs.jobvite.com", "jobvite", modePath},
	{"jobs.quickin.io", "quickin", modePath},
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

	// --- host: board = the whole careers host (regional TLD varies) ---
	{"zohorecruit", "zohorecruit", modeHost},

	// --- hostpath: board = "<host>/<site>" (Workday tenant host + first-path-segment site) ---
	{"myworkdayjobs.com", "workday", modeHostPath},
}

// RecognizeBoard parses a pasted job link into the company board it belongs to: the source
// (ATS provider), the board slug, and the canonical URL to store. ok=false when the host is
// not a supported ATS or the URL carries no board segment/label.
func RecognizeBoard(rawURL string) (source, board, canonical string, ok bool) {
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
		if board == "" {
			return "", "", "", false // bare apex, no tenant label
		}
		// The board IS the host, so the canonical URL is the bare host — collapsing a vacancy
		// URL and the board listing to one board.
		u.RawQuery, u.Fragment, u.Path = "", "", ""
		return src, board, u.String(), true

	case modeHostPath:
		// Workday: board = "<host>/<site>" where site is the first path segment (case-preserved,
		// as the ingest stores it). Canonical strips to scheme://host/site.
		site := firstPathSegment(u)
		if site == "" {
			return "", "", "", false // bare host, no site
		}
		u.RawQuery, u.Fragment = "", ""
		u.Path = "/" + site
		return src, host + "/" + site, u.String(), true

	case modePathLocale:
		// Rippling: skip a leading xx-XX locale segment (ats.rippling.com/en-GB/<board>/…),
		// which the board API omits, and collapse the canonical to the board root so a
		// locale-prefixed vacancy, a bare vacancy, and the listing all map to one board.
		board = boardAfterLocale(u)
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

// boardAfterLocale returns the first path segment that isn't a leading locale — the tenant board
// in ats.rippling.com/<locale?>/<board>/… . "" when the path is empty or carries only a locale.
func boardAfterLocale(u *url.URL) string {
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

// ghNumericID matches a Greenhouse-style numeric job id.
var ghNumericID = regexp.MustCompile(`^[0-9]{7,12}$`)

// greenhouseJobID extracts a Greenhouse job id from a link that carries one but no board token:
// the gh_jid query param (Greenhouse's embed param, e.g. company.com/careers?gh_jid=123), or a
// trailing all-numeric path segment (company.com/careers/…/<id>/). ok=false when neither is
// present. A non-Greenhouse id won't be found downstream, so a false positive is harmless.
func greenhouseJobID(rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	if id := u.Query().Get("gh_jid"); ghNumericID.MatchString(id) {
		return id, true
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	if last := segs[len(segs)-1]; ghNumericID.MatchString(last) {
		return last, true
	}
	return "", false
}

// stripQueryFragment returns rawURL without its query or fragment; the raw string on parse error.
func stripQueryFragment(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.RawQuery, u.Fragment = "", ""
	return u.String()
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
