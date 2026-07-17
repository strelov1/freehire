package contribution

import (
	"net/url"
	"strings"
)

// Board extraction modes. path = the board is the first path segment on a fixed host
// (jobs.lever.co/<board>/…); subdomain = the board is the leftmost DNS label under the apex
// (<board>.recruitee.com). For subdomain the board IS the host, so the canonical URL is the
// bare scheme://host — which collapses a vacancy URL and the board listing to one board.
const (
	modePath      = "path"
	modeSubdomain = "subdomain"
)

// atsBoards lists the supported multi-tenant ATS: a host (exact or subdomain-suffix match) →
// its source key and extraction mode. Hosts were verified against each adapter's public job
// URL. A wrong/missing entry is fail-safe: the link simply isn't recognized (422), never a
// false board. Single-company brands, aggregators, and vanity-domain ATS (Workday/Taleo/
// SuccessFactors/Oracle/…) are deliberately absent — their board can't be derived from a URL.
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
	{"ats.rippling.com", "rippling", modePath},
	{"recruiting.ultipro.com", "ukg", modePath},

	// --- subdomain: board = leftmost DNS label under the apex ---
	{"recruitee.com", "recruitee", modeSubdomain},
	{"bamboohr.com", "bamboohr", modeSubdomain},
	{"breezy.hr", "breezy", modeSubdomain},
	{"freshteam.com", "freshteam", modeSubdomain},
	{"gupy.io", "gupy", modeSubdomain},
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
}

// recognizeBoard parses a pasted job link into the company board it belongs to: the source
// (ATS provider), the board slug, and the canonical URL to store. ok=false when the host is
// not a supported ATS or the URL carries no board segment/label.
func recognizeBoard(rawURL string) (source, board, canonical string, ok bool) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", "", "", false
	}
	host := hostname(u)
	src, mode, apex, known := matchHost(host)
	if !known {
		return "", "", "", false
	}

	if mode == modeSubdomain {
		board = subdomainLabel(host, apex)
		if board == "" {
			return "", "", "", false // bare apex, no tenant label
		}
		// The board IS the host, so the canonical URL is the bare host — collapsing a vacancy
		// URL and the board listing to one board.
		u.RawQuery, u.Fragment, u.Path = "", "", ""
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

// matchHost returns the ATS entry for a host, matching exactly or as a subdomain of the entry
// host. It returns the entry host as the apex so subdomain extraction knows what to strip.
func matchHost(host string) (source, mode, apex string, ok bool) {
	for _, a := range atsBoards {
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
