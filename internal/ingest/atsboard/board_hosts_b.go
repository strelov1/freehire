package atsboard

var atsBoardsB = []struct{ host, source, mode string }{
	// --- subdomainchain: board = every label under the apex (tenant nested under a region) ---
	{"huntflow.io", "huntflow", modeSubdomainChain},

	// --- host: board = the whole careers host (regional TLD varies) ---
	{"zohorecruit", "zohorecruit", modeHost},
	{"teamtailor", "teamtailor", modeHost}, // <tenant>.teamtailor.com; custom-domain career sites are absent (not URL-derivable)
	{"factorial", "factorial", modeHost},   // <tenant>.factorial.<tld>
	{"factorialhr", "factorial", modeHost}, // the .com.br/.pt/… base-domain variant — ONE ingest adapter serves both, and it reports "factorial"
	{"gr8people", "gr8people", modeHost},   // <tenant>.gr8people.com
	{"workgr8", "gr8people", modeHost},     // the workgr8.com base-domain variant — same vendor, confirmed live (identical frontend + GraphQL schema); ONE ingest adapter serves both, reporting "gr8people"
	// Eploy: <tenant>.eploy.net is every tenant's own host and the board the adapter fetches
	// /live-jobs.xml from. A branded careers domain is not derivable, like every other vanity host.
	{"eploy", "eploy", modeHost},
	// d.vinci: <tenant>.dvinci-hr.com (the full product) and <tenant>.dvinci-easy.com (the
	// small-business one) are one application, and the adapter fetches
	// https://<board>/jobPublication/list.json, so the board is the whole host — tenant labels
	// nest (karriere.hahngruppe.dvinci-hr.com). An employer's own domain serves the same path
	// but is not derivable from a URL.
	{"dvinci-hr", "dvinci", modeHost},
	{"dvinci-easy", "dvinci", modeHost},
	{"dvinci", "dvinci", modeHost}, // <tenant>.dvinci.de, the vendor's third domain (bomag.dvinci.de)
	// Talentsoft / Cegid Talent (France): every customer runs one ASP.NET career site on its own
	// host — <tenant>.talent-soft.com, <tenant>.profils.org (the vendor's older domain) or an
	// employer domain — and the adapter reads <board>/offre-de-emploi/liste-toutes-offres.aspx,
	// so the board is the whole host. Labels carry the tenant's own suffix (edf-recrute,
	// framatome-career, dassault-aviation-cand), which is why the host and not a label is kept.
	{"talent-soft.com", "talentsoft", modeHost},
	{"profils.org", "talentsoft", modeHost},
	// DigitalRecruiters (Cegid, France): a career site is a host, almost always the employer's
	// own (careers.segulatechnologies.com) and so not derivable; the few on the vendor's domain
	// are <tenant>-careers.digitalrecruiters.com. The adapter reads the vendor API keyed by that
	// host, so the board is the whole host.
	{"digitalrecruiters.com", "digitalrecruiters", modeHost},
	// Bizneo HR (Spain): a career site is a host under bizneo.cloud — usually
	// <company>.careers.ats.bizneo.cloud, sometimes a vanity label (jobs.abeienergy.bizneo.cloud) —
	// and the adapter lists <board>/jobs, so the board is the whole host. The vendor's bizneo.com
	// hosts carry no live career site.
	{"bizneo.cloud", "bizneo", modeHost},
	// rexx systems: <tenant>-portal.rexx-systems.com and <tenant>-portal.rexx-recruitment.com.
	// The adapter lists <board>/stellenangebote.html, so the board is the whole host. A portal
	// on an employer's own domain (jobs.newyorker.de/karriere) is not derivable from a URL.
	{"rexx-systems", "rexx", modeHost},
	{"rexx-recruitment", "rexx", modeHost},
	// CatsOne's adapter fetches https://<board>/careers, so the board is the whole host, exactly
	// as catsone.yml stores it. It was listed as a subdomain, which yields the bare label — a
	// board no crawl can resolve. (Its custom-domain tenants, e.g. jobs.evoplay.com.ua, stay
	// underivable from a URL, like every other custom-domain ATS here.)
	{"catsone", "catsone", modeHost},
	// Avature's board is the career-site host, the shape avature.yml stores
	// (deloittecm.avature.net). Only tenants on the platform's own domain are derivable; a
	// vanity host (jobs.ea.com) is not, like every other custom-domain ATS here.
	{"avature", "avature", modeHost},
	// Recruiter (Saramin's career-site builder): the adapter sends the whole career host as the
	// API's tenant header, so the board is the host (cosmax.recruiter.co.kr).
	{"recruiter.co.kr", "recruiter_co_kr", modeHost},
	// Abacus Umantis: the adapter lists https://<board>/Jobs/All, so the board is the whole host
	// (recruitingapp-2840.umantis.com). A customer vanity host (jobs.bobst.com) is not derivable
	// from a URL, like every other custom-domain ATS here.
	{"umantis.com", "umantis", modeHost},
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

	// --- hostsite: board = "<host>/<site>" (Paradox: <instance>.paradox.ai/co/<site>/…) ---
	{"paradox.ai", "paradox", modeHostSite},

	// --- allowedfirstsegment: board = the segment after an ALLOWED leading segment (see
	// allowedFirstSegments) — app.dover.com is Dover's whole product SPA, not a dedicated
	// boards host, so a deny-list mode would mint a board out of every unrelated page it serves.
	{"app.dover.com", "dover", modeAllowedFirstSegment},
	// jobs.dover.com is Dover's public job-board host, served by the same search-builder SPA the
	// apply pages are: a company's board is addressed at /<Org>/<uuid>, where the first segment
	// is the company's Dover slug — the SAME identity the app.dover.com /apply and /jobs paths
	// use — and the UUID-shaped second segment is only the shape that proves the link names a
	// board at all (jobs.dover.com/Deepspatial/a5546acf-…, found in the board-submission queue
	// 2026-11-05). That uuid may be the client id OR a posting id — the shell serves any pair
	// to the browser — so it is verified where it is provable, at the API (see the adapter), and
	// the BOARD is the slug alone: folding the uuid in would mint one board per posting and
	// strand listings under ids the careers API does not resolve (the a5546acf link itself is a
	// posting link: its client is 1aeeba55-…). A bare /<Org> is a marketing shell served for ANY
	// slug and stays declined (see pathPatternBoards).
	{"jobs.dover.com", "dover", modePathPattern},
	// HigherMe: app.higherme.com is the whole product too (sign-in, the employer console, the
	// candidate profile), and its posting URL /jobs/<id> names a POSTING, not a brand — the
	// brand behind a posting is only in the posting's API record. Only the brand's career page,
	// /careers/<brand>, names the board.
	{"app.higherme.com", "higherme", modeAllowedFirstSegment},

	// --- tenanthost: board = the whole host, one label under a shared domain ---
	// Feishu Hire's customer careers sites (internal/ingest/sources/feishujobs.go accepts exactly
	// <tenant>.jobs.feishu.cn). ByteDance's own jobs.bytedance.com is a single host, not a tenant
	// space, and is left out.
	{"jobs.feishu.cn", "feishujobs", modeTenantHost},
	// SuccessFactors career sites SAP hosts on its own domain; the adapter reads
	// https://<board>/job_sitemap.xml. Sites on an employer's own domain stay underivable, and so
	// does the legacy careerNN.sapsf.* "career?company=<id>" section, which serves no sitemap.
	{"jobs.hr.cloud.sap", "successfactors", modeTenantHost},
	// SuccessFactors' recruiting-marketing sites on the jobs2web.com domain (assaabloy.jobs2web.com),
	// which serve the same job_sitemap.xml. Staging copies live one label deeper
	// (<tenant>.stage.jobs2web.com) and so are not a tenant; stage. and www. are the vendor's own.
	{"jobs2web.com", "successfactors", modeTenantHost},
	// Phenom career sites on the vendor's own domains — each answers the /widgets API the adapter
	// calls, verified live. A site on the employer's own domain (careers.cisco.com) is not
	// derivable from its URL; boardresolve identifies those from the page.
	{"phenompro.com", "phenom", modeTenantHost},
	{"phenompeople.net", "phenom", modeTenantHost},
	{"phenompeople.com", "phenom", modeTenantHost},
	// Darwinbox career portals: <tenant>.darwinbox.in or <tenant>.darwinbox.com, the TLD being the
	// tenant's data region, so the board is the whole host the adapter posts its listing to. The
	// same host is also the tenant's employee HRMS login, hence the "careers" segment below.
	{"darwinbox.in", "darwinbox", modeTenantHost},
	{"darwinbox.com", "darwinbox", modeTenantHost},
	// greytHR Hire: <tenant>.greythr.com/hire/jobs/, the board being the whole host the adapter
	// reads /hire/api/ on. The same host is the tenant's payroll/HRMS login, hence the "hire"
	// segment below.
	{"greythr.com", "greythr", modeTenantHost},
	// Jibe career sites on the vendor's own domain, <tenant>.jibeapply.com: the adapter's board
	// is the site's whole public host (internal/ingest/sources/jibe.go reads https://<board>/api/jobs,
	// verified live on this domain too).
	{"jibeapply.com", "jibe", modeTenantHost},

	// --- keyedsegment: board = the shaped segment after a fixed key segment ---
	{"workstream.us", "workstream", modeKeyedSegment},
	// LiveHire career sites: /careers/<segment>[…]. The board is the segment; the /jobs and
	// /job/<urlCode>/<advertUrlCode>/<slug> pages under it all name the same tenant, so the
	// canonical collapses to the board's own listing page. www.livehire.com is the vendor's
	// WHOLE product host (dashboards, marketing, API), so anything beyond the fixed /careers
	// prefix is declined rather than read as a board.
	{"livehire.com", "livehire", modeKeyedSegment},

	// --- pathjob: board = the first path segment of a POSTING url, on the apex host only.
	{"harri.com", "harri", modePathJob},

	// --- encodedcareers: board = the base64-encoded numeric id in "/<slug>/<id>/careers" ---
	{"app.idealtraits.com", "idealtraits", modeEncodedCareers},

	// --- pathpattern: board = the capture group of a per-host path pattern (see pathPatternBoards) ---
	{"paycomonline.net", "paycom", modePathPattern},
	{"comeet.com", "comeet", modePathPattern},
	{"comeet.co", "comeet", modePathPattern}, // 301s to comeet.com with the path unchanged
	{"governmentjobs.com", "neogov", modePathPattern},
	{"schooljobs.com", "neogov", modePathPattern},
	{"geekhunter.com", "geekhunter", modePathPattern},
	{"cleverstaff.net", "cleverstaff", modePathPattern},
	{"careers.flatchr.io", "flatchr", modePathPattern},
	{"app.trinethire.com", "trinethire", modePathPattern},
	{"jobs.gohire.io", "gohire", modePathPattern},
	{"recruiterflow.com", "recruiterflow", modePathPattern},
	// The rows below carry the tenant in the HOST as well as the path (tenantHost), so the
	// board is "<host>/<capture>". Each was read off the adapter's own board parser.
	{"taleo.net", "taleo", modePathPattern}, // after tbe.taleo.net (taleobe) above, which wins first
	{"oraclecloud.com", "oracle", modePathPattern},
	{"app.loxo.co", "loxo", modePathPattern},
	// Velora HR (Spain): <tenant>.career.velorahr.com/<slug>-<career-site id>/…; the board is the
	// id, since one tenant runs several sites (see pathPatternBoards).
	{"career.velorahr.com", "velorahr", modePathPattern},
}
