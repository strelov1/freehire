package atsboard

import (
	"net/url"
	"regexp"
	"slices"
	"strings"
)

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
	"hrmdirect.com":   {"apply"},
	"recruiter.co.kr": {"api-recruiter", "www"}, // api-recruiter is the host the adapter reads, not a tenant
	"potok.io":        {"www", "stages", "qa", "sentry"},
	"jobs2web.com":    {"www", "stage", "feeds"},
	"eploy":           {"www"}, // www.eploy.com is the vendor's marketing site
	"dvinci-hr":       {"www"},
	"dvinci-easy":     {"www"},
	"dvinci":          {"www"}, // www.dvinci.de is the vendor\'s marketing site
	// www is the vendor's marketing site; cdn serves every portal's shared assets.
	"rexx-systems":     {"www", "cdn"},
	"rexx-recruitment": {"www", "cdn"},
	"hiringroom.com":   {"www", "ve", "api-ve", "contacto-comercial"},
	"recruiterbox.com": {"www", "developers", "go"}, // the vendor's marketing, API docs and campaign hosts
	"easyapply.co":     {"www"},                     // www.easyapply.co 404s; every tenant is a bare label
	// The vendor's marketing, training and help-centre hosts, and the CNAME target every
	// employer domain points at.
	"digitalrecruiters.com": {"www", "resources", "academy", "external-career", "status"},
	// www is the vendor's marketing site; demo is its sample board for a fictional "Acme".
	"jobspage.co": {"www", "demo"},
	// www is the vendor's marketing site (an unknown tenant redirects to its "job-not-found"
	// page); www2 and public serve the shared assets and logos every tenant page links.
	"applicantstack.com": {"www", "www2", "public"},
	// www.hireclick.com redirects to employers.hireclick.com, the vendor's marketing site —
	// which is also where an unknown tenant label lands.
	"hireclick.com": {"www", "employers"},
	// Every unknown gupy.io label answers as an (empty) career page, so these are the vendor's
	// hosts that resolve somewhere ELSE: the marketing site and its content hubs, the candidates'
	// job-search portal, the API the adapter reads, support, community, docs and status.
	"gupy.io":      {"www", "portal", "private-api", "suporte", "comunidade", "developers", "status", "conteudo", "academy"},
	"eightfold.ai": {"www"}, // the vendor's marketing site
	// www is the vendor's marketing site; "empregos" is the platform's own bucket for employers
	// that post under "Empresa com nome confidencial" — many employers, none named, so not a
	// board any one company owns.
	"pandape.infojobs.com.br":  {"www", "empregos"},
	"pandape.catho.com.br":     {"www", "empregos"},
	"pandape.com.br":           {"www", "empregos"},
	"pandape.computrabajo.com": {"www", "empleos"},
	"coveto.de":                {"www"}, // the vendor's marketing site and blog
	"concludis.de":             {"www"},
	// Kenjo's marketing site, its HR-templates hub, and the host signed URLs to tenant files
	// are served from; help, blog and app are platformLabels already.
	"kenjo.io": {"www", "hr", "files"},
	// talentstorm's marketing site, its API, and the host its tutorial videos are served from.
	"talentstorm.de": {"www", "api", "videos"},
	// Homerun's marketing site and sign-up funnel, the host a gone tenant's pages redirect to, the
	// one feed host every tenant's feed is served from, and its asset hosts.
	"homerun.co":    {"www", "try", "404", "feed", "static", "cdn"},
	"easycruit.com": {"www"}, // Visma's EasyCruit product site
	// Firefish's marketing site; jobs is the vendor's own careers site (which runs the same
	// RSS feed as every tenant's, and is a crawlerable source of ONE — the vendor's).
	"firefishsoftware.com": {"www", "jobs"},
	// Recruitify's marketing site, its API, and the vendor's video-interview and sourcing products.
	"recruitify.ai": {"www", "api", "iview", "glb", "sourcingnow"},
	// Oorwin's marketing site and the vendor's own demo/CRM portals, which answer the careers API
	// with sample postings.
	"oorwin.com":     {"www", "demous", "demoind", "demosg", "oorwincrm"},
	"inhroffice.com": {"www", "link"},          // link.inhroffice.com is the vendor's link shortener
	"epreselec.com":  {"www", "logininfojobs"}, // the product site and the InfoJobs sign-in bridge
	"viterbit.site":  {"www"},
	// Darwinbox's marketing site, its content hub and its events site.
	"darwinbox.com": {"www", "explore", "lighthouse"},
	"darwinbox.in":  {"www"},
	// greytHR's marketing site, community forum, help centres, status page and campaign pages.
	"greythr.com":  {"www", "community", "admin-help", "ess-help", "status", "campaign"},
	"hirehive.com": {"www", "assets", "static", "hirehive-testing-account"},
}

// platformHost reports whether host is the ATS's own product host rather than a tenant's —
// either a label no multi-tenant ATS gives a tenant, or one this platform in particular keeps.
func platformHost(host, apex string) bool {
	label, _, ok := strings.Cut(host, ".")
	if !ok {
		return false
	}
	if p := platformLabelPatternsByApex[apex]; p != nil && p.MatchString(label) {
		return true
	}
	return platformLabels[label] || slices.Contains(platformLabelsByApex[apex], label)
}

// platformLabelPatternsByApex is platformLabelsByApex for vendor hosts that come numbered, which a
// fixed list cannot name: SuccessFactors runs its production data centres' Command Center as
// bizx<N>.jobs2web.com (bizx12.jobs2web.com/commandcenter) and serves recruiting-marketing map
// tiles from rmk-map-<N>.jobs2web.com — neither is a customer's career site.
var platformLabelPatternsByApex = map[string]*regexp.Regexp{
	"jobs2web.com": regexp.MustCompile(`^(?:bizx[0-9]*|rmk-map-[0-9]+)$`),
}

// paradoxSite matches a Paradox career-site slug ("ChickenSaladChick1", "2ndFamily28").
var paradoxSite = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

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

// bareLanguage matches a two-letter language code standing alone as a path segment ("es").
var bareLanguage = regexp.MustCompile(`^[a-z]{2}$`)

// workdaySite returns the career site a Workday URL addresses, "" when it names none. Beside the
// xx-XX locale every reader skips, Workday also serves a posting under a BARE language,
// host/es/<site>/job/… — measured 2026-10-27 on Spanish employers' apply links (Repsol, 3M,
// Europcar), where reading "es" as the site yields "<host>/es", a board that does not exist. A
// bare two-letter segment cannot be skipped unconditionally: ~80 live boards ARE two-letter sites
// (pt, se, us, ch, …). So it is read as a language only when the posting marker follows the NEXT
// segment — host/es/<site>/job/… — since a two-letter site's own posting reads host/pt/job/….
func workdaySite(u *url.URL) string {
	segs := segmentsAfterLocale(u)
	if len(segs) >= 3 && bareLanguage.MatchString(segs[0]) && (segs[2] == "job" || segs[2] == "details") {
		segs = segs[1:]
	}
	if len(segs) == 0 {
		return ""
	}
	return segs[0]
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
