package atsboard

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
	// jobappnetwork (talentReef): the platform's own public apply link IS this shape
	// (apply.jobappnetwork.com/clients/<clientId>/posting/<id>/…), so a person-pasted or
	// harvested URL resolves through the same mechanism as the XHR-only hosts above.
	{"apply.jobappnetwork.com", "jobappnetwork", "clients"},
	// Paylocity serves every tenant from one host, and only the LISTING names the board:
	// /Recruiting/Jobs/All/<board>. A posting (/Recruiting/Jobs/Details/<id>) and its apply
	// form (/Recruiting/Jobs/Apply/<id>) carry a posting id and nothing else, so they match no
	// prefix here and are declined — which is right, since skipping the segment would take the
	// posting id for a board, the same trap workable's "/j/<id>" shortlink carries.
	{"recruiting.paylocity.com", "paylocity", "Recruiting/Jobs/All"},
	// HR-Manager's job-portal API, which a customer's own careers page calls to list its
	// postings: /JobPortal.svc/<alias>/PositionList/json/ (embeds spell the path in either
	// case, hence the case-insensitive prefix). The alias is the board the adapter reads.
	{"api.hr-manager.net", "hrmanager", "JobPortal.svc"},
	// HiringRoom's embeddable vacancy list: an employer that keeps its careers page on its own
	// domain loads hiringroom.com/assets/js/api/embed.js, which frames
	// /api/vacancies/get_vacancy_list_frame/<tenant>. That frame is the only place such a page
	// names the tenant; the bare apex is otherwise the vendor's own host.
	{"hiringroom.com", "hiringroom", "api/vacancies/get_vacancy_list_frame"},
}

// noBoardHosts are hosts under a supported ATS's domain that serve the platform's own
// infrastructure rather than any tenant, so no path on them names a board. platformLabels
// cannot reach these: it reads the LEFTMOST label, and job-boards.cdn.greenhouse.io leads with
// the same "job-boards" the real board hosts do — what makes it infrastructure sits in the
// middle. A full-host entry is also the stabler statement: a CDN is not an employer whatever
// it renames its asset folder to, whereas listing the folder ("assets") only holds until the
// next bundler change.
//
// This matters because boardresolve takes the FIRST recognized ATS URL in a fetched page, and
// every Greenhouse job page loads its stylesheet from this host in <head> — well before the
// link to the board itself. A pasted vanity or grnh.se link therefore resolved to the board
// "assets" instead of the employer (this cost lionhires, 27 postings, one rejected row).
var noBoardHosts = map[string]bool{
	"job-boards.cdn.greenhouse.io": true,
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
	// CHECK-IN's own machinery on the tenant hosts (the login app, the minified assets), and
	// "/detail/<uuid>", the posting path of "<name>-karriereportal.mein-check-in.de" — a different
	// CHECK-IN product the meincheckin adapter cannot read.
	"mein-check-in.de":   {"bm-login", "min", "detail"},
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
	// Greenhouse's AI-screening opt-out form is ONE shared endpoint
	// (app4.greenhouse.io/ai_opt_out_request/job_post/<id>/ai_opt_out), served identically to
	// every customer, and a career page carries its URL in markup that names the employer's own
	// board nowhere else. It belongs here rather than in reservedSegments because the segment
	// behind it is the platform's own "job_post": skipping would move the false board one
	// segment along, not decline it. Two unrelated employers resolving to the same board is what
	// surfaced it (2026-09-08).
	"greenhouse.io": {"ai_opt_out_request"},
	// ADP MyJobs' own API (/public/staffing/v1/career-site/<board>, which the adapter calls)
	// shares the host with the career sites; "public" is not a tenant.
	"myjobs.adp.com": {"public"},
	// The AppOne portal SPA loads its own API, bundle assets and uploaded logos from the host its
	// portals live on; none of them is an employer.
	"jobs.appone.com": {"api", "assets", "company-images"},
	// Rippling's "/jobs/<posting-uuid>" carries no tenant: it 308s to /rippling/jobs/<uuid> for
	// ANY uuid, real or not (verified live 2026-10-25), so skipping "jobs" would take the POSTING
	// id as the board and keeping it would file a board called "jobs".
	"ats.rippling.com": {"jobs"},
}

// noBoardTenantSegments lists, per matched host entry in modePathPair, the words that sit where
// the TENANT does (<locale>/<word>) but are the platform's own pages. werecruit's
// /<locale>/redirectToPage is its post-login redirect: read as a board it is "en/redirectToPage",
// whose probe 404s (found in the French employer harvest, 2026-10-28). Compared lower-cased.
var noBoardTenantSegments = map[string][]string{
	"careers.werecruit.io": {"redirecttopage"},
}

// allowedFirstSegments lists, per matched host entry in modeAllowedFirstSegment, the ONLY leading
// path segments that carry a board behind them — the inverse of reservedSegments: everything NOT
// in this list is declined outright rather than skipped to reach a tenant. Dover's job board and
// apply pages are the only two path shapes this covers; every other app.dover.com page (the
// dashboard, /crm, /docs/api, sign-in) leads with something else and is correctly declined.
//
// listing is the leading segment of the board's own listing page, which the canonical collapses
// to, so a posting link and the listing map to one board.
var allowedFirstSegments = map[string]struct {
	segments []string
	listing  string
}{
	"app.dover.com":    {segments: []string{"apply", "jobs"}, listing: "jobs"},
	"app.higherme.com": {segments: []string{"careers"}, listing: "careers"},
}

var reservedSegments = map[string][]string{
	"jobs.jobvite.com": {"careers"},
	// HERP addresses every board at /v1/<board>/…; "v1" is the platform's own path word.
	"herp.careers": {"v1"},
	// HRMOS addresses every board at /pages/<board>/…; "pages" is the platform's own path word.
	"hrmos.co":      {"pages"},
	"greenhouse.io": {"embed", "job_app", "job_board", "js"},
	// Gusto's board listing is /boards/<board>; "boards" is the platform's word, never a tenant.
	"jobs.gusto.com": {"boards"},
	// JobScore: /careers/<board>/… (the career site) and /jobs/<board>/feed.json (the feed).
	"careers.jobscore.com": {"careers", "jobs"},
}
