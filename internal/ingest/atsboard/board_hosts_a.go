package atsboard

// atsBoardsA and atsBoardsB are the two ordered halves of atsBoards (see there); the order
// across the cut is the order of the table, A first.
var atsBoardsA = []struct{ host, source, mode string }{
	// --- path: board = first path segment on a fixed host ---
	{"greenhouse.io", "greenhouse", modePath},
	{"jobs.lever.co", "lever", modePath},
	{"jobs.eu.lever.co", "lever", modePath}, // Lever EU data-residency host; same path shape/board as the US host
	{"jobs.ashbyhq.com", "ashby", modePath},
	{"jobs.humanbit.ai", "humanbit", modePath},
	{"jobs.recrutei.com.br", "recrutei", modePath},
	{"jobs.pyjamahr.com", "pyjamahr", modePath},
	{"apply.workable.com", "workable", modePath},
	{"jobs.deel.com", "deel", modePath},
	// CHECK-IN (mein-check-in.de): a tenant is the first path segment on one of three branded
	// hosts (www., sparkasse., vr.), /<tenant>/position-<id>.
	{"mein-check-in.de", "meincheckin", modePath},
	{"jobs.gem.com", "gem", modePath},
	{"jobs.friend.work", "friendwork", modePath}, // FriendWork career portal: jobs.friend.work/<token>/<id>
	{"jobs.jobvite.com", "jobvite", modePath},
	// HERP's board sits at /v1/<board>/… — "v1" is platform machinery, reserved below, the
	// same shape Gusto's "boards" word already uses.
	{"herp.careers", "herp", modePath},
	// HRMOS's board sits at /pages/<board>/… — "pages" is platform machinery, reserved below,
	// the same shape Gusto's "boards" word already uses.
	{"hrmos.co", "hrmos", modePath},
	{"jobs.quickin.io", "quickin", modePath},
	{"jobs.talenthr.io", "talenthr", modePath},
	{"jobs.polymer.co", "polymer", modePath}, // Polymer: jobs.polymer.co/<board>/<numeric id>
	{"careers.pageuppeople.com", "pageup", modePathNumeric},
	{"oportunidades.mindsight.com.br", "mindsight", modePath},
	{"careers.hireology.com", "hireology", modePath},
	// Hireology's newer career-site host, <tenant>.hireology.careers: the label is the same tenant
	// slug the adapter's api.hireology.com/v1/careers/<slug> takes (verified live).
	{"hireology.careers", "hireology", modeSubdomain},
	// ADP MyJobs: myjobs.adp.com/<board>/cx/…. A different ADP product from Workforce Now
	// (modeQueryPair below) — its board is the career site's own slug, which the adpmyjobs
	// adapter resolves to the orgoid its listing needs; neither id derives from the other.
	{"myjobs.adp.com", "adpmyjobs", modePath},
	// AppliTrack serves every district from one host: www.applitrack.com/<board>/onlineapp/….
	{"applitrack.com", "applitrack", modePath},
	// Manatal's hosted career-page domain serves a tenant at careers-page.com/<tenant>/job/<id>
	// AND at <tenant>.careers-page.com (pathTenantSubdomains), and its source is manatal:
	// careerspage.yml is deliberately empty because every careers-page.com tenant is served by
	// the Manatal adapter.
	{"careers-page.com", "manatal", modePath},
	// Gusto Hiring serves a board at /boards/<board>, where the board is the WHOLE
	// "<company-slug>-<company-uuid>" segment — neither half resolves alone. "boards" is the
	// platform's own path word (reservedSegments), so it is skipped to reach the board behind it.
	// A /postings/<…> URL is declined (noBoardFirstSegments): its segment ends in the POSTING's
	// uuid and carries the job's title slug, so it names no board at all — the only link back to
	// one is the breadcrumb the posting page renders, which is boardresolve's job, not a URL's.
	{"jobs.gusto.com", "gusto", modePath},
	// AppOne (Paychex) serves every employer's job portal at jobs.appone.com/<slug>. Its posting
	// URLs (apply.appone.com/job/<id>) are deliberately absent: they name the posting, never the
	// portal, which only the posting's own API record carries (internal/ingest/sources/appone.go).
	{"jobs.appone.com", "appone", modePath},
	// JobScore: careers.jobscore.com/careers/<board>/jobs/<posting> and the feed the adapter
	// reads, /jobs/<board>/feed.json. Both leading words are the platform's (reservedSegments).
	{"careers.jobscore.com", "jobscore", modePath},

	// --- pathportal: board = the segment before the posting segment ---
	{"jobs.smartrecruiters.com", "smartrecruiters", modePathPortal},
	{"careers.smartrecruiters.com", "smartrecruiters", modePathPortal},

	// --- pathlocale: like path, skipping a leading xx-XX locale segment ---
	{"ats.rippling.com", "rippling", modePathLocale},

	// --- pathlocalepair: board = the two path segments after an optional leading locale ---
	{"jobs.dayforcehcm.com", "dayforce", modePathLocalePair},

	// --- pathpair: board = the first two path segments, verbatim (the leading one is a
	// REQUIRED locale, not an optional one to fold off) ---
	{"careers.werecruit.io", "werecruit", modePathPair},

	// --- query: board = a named query parameter (see queryBoards) ---
	{"recruitingbypaycor.com", "paycor", modeQuery},
	{"candidate.hr-manager.net", "hrmanager", modeQuery},
	{"pulse.tenstreet.com", "tenstreet", modeQuery},
	{"skk.erecruiter.pl", "erecruiter", modeQuery},
	// Eightfold tenants on the vendor's own domain (<tenant>.eightfold.ai). A career site on the
	// employer's own domain (apply.careers.microsoft.com) is a vanity host and stays unrecognised.
	{"eightfold.ai", "eightfold", modeQuery},

	// --- querypair: board = two named query parameters joined by ":" (see queryPairBoards) ---
	{"workforcenow.adp.com", "adp", modeQueryPair},
	// Taleo Business Edition, on every pod host (<pod>.tbe.taleo.net). Taleo Enterprise tenants
	// (<tenant>.taleo.net) are a different product and stay unmatched: this entry only takes
	// hosts under tbe.taleo.net.
	{"tbe.taleo.net", "taleobe", modeQueryPair},

	// --- icims: board = bare slug (classic "careers-<slug>" host) or the whole vanity host ---
	{"icims.com", "icims", modeICIMS},

	// --- subdomain: board = leftmost DNS label under the apex ---
	{"betterteam.com", "betterteam", modeSubdomain},
	{"recruitee.com", "recruitee", modeSubdomain},
	{"bamboohr.com", "bamboohr", modeSubdomain},
	{"breezy.hr", "breezy", modeSubdomain},
	{"jobspage.co", "jobspage", modeSubdomain},
	// onlyfy by XING (formerly prescreen): <board>.onlyfy.jobs, and the older <board>.jobbase.io,
	// which redirects to the same site.
	{"onlyfy.jobs", "onlyfy", modeSubdomain},
	{"jobbase.io", "onlyfy", modeSubdomain},
	{"freshteam.com", "freshteam", modeSubdomain},
	{"peopleforce.io", "peopleforce", modeSubdomain},
	// Potok: <tenant>.potok.io/open/jobs/<id>. Its own infrastructure hosts share the wildcard
	// certificate, so they are listed in platformLabelsByApex.
	{"potok.io", "potok", modeSubdomain},
	{"jobs.personio.com", "personio", modeSubdomain},
	{"jobs.personio.de", "personio", modeSubdomain}, // Personio DE regional host; board = same tenant subdomain
	{"pinpointhq.com", "pinpoint", modeSubdomain},
	{"scalis.ai", "scalis", modeSubdomain},
	{"keka.com", "keka", modeSubdomain},
	{"hirehive.com", "hirehive", modeSubdomain},
	{"app.beehire.com", "beehire", modePathPattern},
	{"inploi.com", "inploi", modePathPattern},
	{"seamlesshiring.com", "seamlesshiring", modeSubdomain},
	{"selfrecruit.ge", "selfrecruit", modeSubdomain},
	{"talentlyft.com", "talentlyft", modeSubdomain},
	{"traffit.com", "traffit", modeSubdomain},
	// Recruitify (Polish): the portal is a label on recruitify.ai (linkgroup.recruitify.ai/job/<guid>).
	// Vendor hosts are in platformLabelsByApex.
	{"recruitify.ai", "recruitify", modeSubdomain},
	{"applytojob.com", "jazzhr", modeSubdomain},
	{"applicantpro.com", "applicantpro", modeSubdomain},
	{"isolvedhire.com", "isolvedhire", modeSubdomain},
	{"careerplug.com", "careerplug", modeSubdomain},
	// ApplicantStack: <tenant>.applicantstack.com/x/detail/<id>. The vendor's own www/www2/public
	// hosts share the apex, so they are listed in platformLabelsByApex.
	{"applicantstack.com", "applicantstack", modeSubdomain},
	// symplr's career centers: <tenant>.hcshiring.com, the host the adapter reads the tenant's
	// API at. The vendor's other hosts (hctsportals.com, pm.healthcaresource.com) serve
	// different products that adapter does not read, so they stay unrecognised.
	{"hcshiring.com", "hcshiring", modeSubdomain},
	{"csod.com", "cornerstone", modeSubdomain},
	{"enlizt.me", "enlizt", modeSubdomain},
	{"hurma.work", "hurma", modeSubdomain},
	{"inhire.app", "inhire", modeSubdomain},
	{"likeit.fi", "likeit", modeSubdomain},
	{"spark.work", "spark", modeSubdomain},
	{"hire.trakstar.com", "trakstar", modeSubdomain},
	// Recruiterbox is Trakstar Hire's former name: <tenant>.recruiterbox.com still 301s to
	// <tenant>.hire.trakstar.com under the SAME label, and older careers pages link the old host.
	{"recruiterbox.com", "trakstar", modeSubdomain},
	// GetHired: <tenant>.easyapply.co is the career site the adapter reads /rss from. Its
	// apply links (easyapply.co/a/<uuid>, easyapply.co/job/<slug>) sit on the bare apex and
	// name no tenant, so they stay unrecognised — the board is only on the career site.
	{"easyapply.co", "gethired", modeSubdomain},
	// Firefish (firefishsoftware.com): every tenant career site is a subdomain of the vendor's
	// domain, and the board is that WHOLE host — the address the tenant's postings carry and the
	// host the adapter reads the RSS feed at. Host mode, not subdomain: a tenant label is not a
	// usable id, its own careers domain is. The vendor's own site is jobs.firefishsoftware.com,
	// and unknown labels NXDOMAIN rather than serve a page, so nothing else on the domain reads
	// as a tenant; www, the marketing site, is declined through the label list below.
	{"firefishsoftware.com", "firefish", modeHost},
	{"portaldetalentos.senior.com.br", "senior", modeSubdomain},
	{"vagas.solides.com.br", "solides", modeSubdomain},
	{"softgarden.io", "softgarden", modeSubdomain},
	// softgarden's regional career host. The tenant label is the same board the adapter fetches
	// at <board>.softgarden.io, so this apex only changes where the label sits — keyed on
	// softgarden.io alone, a .de tenant hid behind a second DNS label and was declined.
	{"career.softgarden.de", "softgarden", modeSubdomain},
	{"careers.hibob.com", "hibob", modeSubdomain}, // HiBob's careers module: <tenant>.careers.hibob.com
	// German ATSs whose tenants are a label on the vendor's domain. coveto's label is the
	// customer number (k60360.coveto.de); concludis's and Kenjo's are a name
	// (hhla.concludis.de, ict-ds.kenjo.io). The vendors' own hosts are in platformLabelsByApex.
	{"coveto.de", "coveto", modeSubdomain},
	{"concludis.de", "concludis", modeSubdomain},
	{"kenjo.io", "kenjo", modeSubdomain},
	{"talentstorm.de", "talentstorm", modeSubdomain},
	// Homerun (Dutch) and Visma EasyCruit (Dutch and Nordic): the tenant is a label on the vendor's
	// domain (douglas.homerun.co, lentis.easycruit.com). A Homerun site often lives on the
	// employer's own domain instead, which no URL rule can name. Vendor hosts are in
	// platformLabelsByApex.
	{"homerun.co", "homerun", modeSubdomain},
	{"easycruit.com", "easycruit", modeSubdomain},
	// Oorwin (US/Indian IT staffing ATS): the career portal is a label on oorwin.com
	// (purpledrive.oorwin.com/careers), the label the adapter names in its API body. Vendor
	// hosts and demo tenants are in platformLabelsByApex.
	{"oorwin.com", "oorwin", modeSubdomain},
	// HR Office (Dutch): the career site is a label on inhroffice.com. Employers that show the
	// same vacancies on their own domain pull them from there, so those domains name no board.
	{"inhroffice.com", "hroffice", modeSubdomain},
	// ePreselec (Spain, Adevinta): one ASP.NET careers site per customer at <label>.epreselec.com.
	{"epreselec.com", "epreselec", modeSubdomain},
	// Viterbit (Spain): one career site per customer at <label>.viterbit.site.
	{"viterbit.site", "viterbit", modeSubdomain},
	// helixjobs: the concludis portal, keyed by instance id (see queryBoards).
	{"helixjobs.com", "helixjobs", modeQuery},
	// CVWarehouse: every customer's career site is jobpage.cvwarehouse.com/?companyGuid=<board>.
	{"jobpage.cvwarehouse.com", "cvwarehouse", modeQuery},
	// Emply: <tenant>.career.emply.com. A site on the customer's own domain (midtjob.dk) is a
	// vanity host and stays unrecognised here.
	{"career.emply.com", "emply", modeSubdomain},
	// HRM Direct's career site is <board>.hrmdirect.com, exactly what hrmdirect.yml stores. Its
	// application forms live on the platform's own apply.hrmdirect.com — see this apex's entry
	// in platformLabelsByApex, without which every apply link on the platform names a board.
	{"hrmdirect.com", "hrmdirect", modeSubdomain},
	// ...which now redirects to ClearCompany's apply.clearcompany.com/<board>, same slug.
	{"apply.clearcompany.com", "hrmdirect", modePathPattern},
	// HireClick: <tenant>.hireclick.com/jb/<slug>/view/<id>; the adapter fetches the tenant's
	// job board at <board>.hireclick.com. The vendor's own hosts are in platformLabelsByApex.
	{"hireclick.com", "hireclick", modeSubdomain},
	// HiringRoom: one <tenant>.hiringroom.com portal per employer; the board is the label. The
	// vendor's own hosts (marketing, the Venezuelan instance and its API, a sales form) are in
	// platformLabelsByApex.
	{"hiringroom.com", "hiringroom", modeSubdomain},
	// Gupy: one <subdomain>.gupy.io career page per employer, the subdomain the adapter lists
	// by (?subdomain=<board>). The vendor's own hosts are in platformLabelsByApex.
	{"gupy.io", "gupy", modeSubdomain},
	// Intervieweb (Zucchetti): <tenant>.intervieweb.it. The board is the whole careers host, the
	// shape the adapter fetches (a customer may also front a tenant with its own domain, which
	// is not URL-derivable and stays unrecognised).
	{"intervieweb", "intervieweb", modeHost},
	// Pandapé (InfoJobs Brasil's ATS): one <tenant>.pandape.infojobs.com.br career site per
	// employer. The same tenant answers on the Catho and pandape.com.br domains with the same
	// listing, so all three name one board. The vendor's own hosts are in platformLabelsByApex.
	{"pandape.infojobs.com.br", "pandape", modeSubdomain},
	{"pandape.catho.com.br", "pandape", modeSubdomain},
	{"pandape.com.br", "pandape", modeSubdomain},
	// Pandapé LatAm (Computrabajo's deployment of the same ATS): <tenant>.pandape.computrabajo.com,
	// a tenant namespace of its own — a Brazilian tenant 404s here — hence its own provider.
	{"pandape.computrabajo.com", "pandapelatam", modeSubdomain},
}
