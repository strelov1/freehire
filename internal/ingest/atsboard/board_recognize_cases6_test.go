package atsboard

// recognizeCases6 is part 6 of 7 of the TestRecognize table, in order.
var recognizeCases6 = []recognizeCase{
	// Dover: app.dover.com is the whole product SPA (dashboard, /crm, /docs/api, login), not a
	// dedicated boards host, so only an ALLOWED leading segment ("apply"/"jobs") carries a
	// board — everything else is declined outright, the inverse of every deny-list
	// (reservedSegments) rule above.
	{"dover apply link", "https://app.dover.com/apply/qompyl/f9fd123e-e7ff-4bcc-a16a-bb8c47e591b5", "dover", "qompyl", "https://app.dover.com/jobs/qompyl", true},
	{"dover board listing", "https://app.dover.com/jobs/qompyl", "dover", "qompyl", "https://app.dover.com/jobs/qompyl", true},

	// Harri: harri.com/<brand slug>/job/<id>-<title>, read only off a posting URL on the apex
	// host, because the same host serves the vendor's own pages. The first two are real apply
	// links (their title suffix arrives empty); the slug is lower-cased, as the catalogue
	// stores it and the platform matches it case-blind.
	{"harri apply link", "https://harri.com/panera-careers/job/2843465-", "harri", "panera-careers", "https://harri.com/panera-careers", true},
	{"harri apply link, trailing-hyphen slug", "https://harri.com/tig-reaper-llc-/job/2822379-", "harri", "tig-reaper-llc-", "https://harri.com/tig-reaper-llc-", true},
	{"harri share link keeps a store slug, lower-cased", "https://www.harri.com/Taco-Social/job/2793867-cashier?utm_source=x", "harri", "taco-social", "https://www.harri.com/taco-social", true},
	{"harri career page alone is not read", "https://harri.com/bicos", "", "", "", false},
	{"harri vendor page is not a brand", "https://harri.com/employee-records", "", "", "", false},
	{"harri job segment without an id", "https://harri.com/search/job/cashier", "", "", "", false},
	{"harri api host is not a brand", "https://gateway.harri.com/core-reader/job/2845321/x", "", "", "", false},
	{"harri marketing subdomain is not a brand", "https://solutions.harri.com/demo/job/1", "", "", "", false},

	// Betterteam: the tenant is the subdomain (<board>.betterteam.com/<posting-slug>).
	{"betterteam posting", "https://110grill.betterteam.com/bartender-20", "betterteam", "110grill", "https://110grill.betterteam.com", true},
	{"betterteam marketing site has no board", "https://www.betterteam.com/job-posting", "", "", "", false},
	{"dover pricing page carries no board", "https://app.dover.com/pricing", "", "", "", false},
	{"dover login page carries no board", "https://app.dover.com/login", "", "", "", false},
	{"dover crm dashboard carries no board", "https://app.dover.com/crm", "", "", "", false},
	{"dover api docs carry no board", "https://app.dover.com/docs/api", "", "", "", false},
	{"dover bare apply has no board", "https://app.dover.com/apply", "", "", "", false},
	{"dover bare jobs has no board", "https://app.dover.com/jobs", "", "", "", false},

	// tenanthost: the whole host, one label under the platform's shared domain. Every
	// recognised URL here is a real apply link from an external harvest.
	{"feishu tenant posting", "https://aibee.jobs.feishu.cn/index/position/7696734978956233023/detail", "feishujobs", "aibee.jobs.feishu.cn", "https://aibee.jobs.feishu.cn", true},
	{"feishu tenant posting under a site code", "https://poizon.jobs.feishu.cn/089488/position/7682629034660104490/detail", "feishujobs", "poizon.jobs.feishu.cn", "https://poizon.jobs.feishu.cn", true},
	{"feishu bare shared domain has no board", "https://jobs.feishu.cn/", "", "", "", false},
	{"feishu nested host is not a tenant", "https://a.b.jobs.feishu.cn/index", "", "", "", false},
	{"successfactors sap-hosted site", "https://mhh.jobs.hr.cloud.sap/job/Hannover-Duales-Studium-Business-Economics-%28wdm%29-NI-30625/1373094055/", "successfactors", "mhh.jobs.hr.cloud.sap", "https://mhh.jobs.hr.cloud.sap", true},
	{"successfactors jobs2web site", "https://assaabloy.jobs2web.com/job/Zurich-Software-Engineer/1234567801/", "successfactors", "assaabloy.jobs2web.com", "https://assaabloy.jobs2web.com", true},
	{"successfactors jobs2web staging site is not a board", "https://assaabloy.stage.jobs2web.com/job/Zurich-Software-Engineer/1234567801/", "", "", "", false},
	{"successfactors jobs2web bare staging host is not a board", "https://stage.jobs2web.com/", "", "", "", false},
	{"successfactors jobs2web vendor host is not a board", "https://www.jobs2web.com/", "", "", "", false},
	{"successfactors jobs2web command center is not a board", "https://bizx12.jobs2web.com/commandcenter", "", "", "", false},
	{"successfactors jobs2web map tiles are not a board", "https://rmk-map-12.jobs2web.com/", "", "", "", false},
	{"successfactors jobs2web feeds host is not a board", "https://feeds.jobs2web.com/", "", "", "", false},
	{"successfactors legacy career section is not derivable", "https://career55.sapsf.eu/career?career_ns=job_listing&company=pressglass&career_job_req_id=1395&lang=pl_PL", "", "", "", false},
	{"phenom phenompro posting", "https://qualtrics.phenompro.com/us/en/job/8274275/State-and-Local-Government-Sales-Manager", "phenom", "qualtrics.phenompro.com", "https://qualtrics.phenompro.com", true},
	{"phenom phenompeople.net posting", "https://acadiamigration.phenompeople.net/us/en/job/110212/Substance-Use-Counselor-TCADC-CADC", "phenom", "acadiamigration.phenompeople.net", "https://acadiamigration.phenompeople.net", true},
	{"phenom phenompeople.com posting without a locale", "https://lifetime.phenompeople.com/job/R-174914", "phenom", "lifetime.phenompeople.com", "https://lifetime.phenompeople.com", true},
	// Phenom serves its CDN from the same domains; a career page links there on every load.
	{"phenom cdn asset is not a board", "https://cdn.phenompeople.com/CareerConnectResources/pp/QUALUS/images/logo.png", "", "", "", false},
	{"phenom vendor site is not a board", "https://www.phenompeople.com/", "", "", "", false},
	{"darwinbox v2 posting", "https://atherenergy.darwinbox.in/ms/candidatev2/main/careers/jobDetails/a6ad892cd96ba1", "darwinbox", "atherenergy.darwinbox.in", "https://atherenergy.darwinbox.in", true},
	{"darwinbox v1 careers on the .com region", "https://carsome.darwinbox.com/ms/candidate/careers/a665ee161adc9c", "darwinbox", "carsome.darwinbox.com", "https://carsome.darwinbox.com", true},
	{"greythr hire posting", "https://specsmakershrm.greythr.com/hire/jobs/optometrist-147", "greythr", "specsmakershrm.greythr.com", "https://specsmakershrm.greythr.com", true},
	{"greythr HRMS login is not a careers board", "https://aaryak.greythr.com/uas/portal/auth/login", "", "", "", false},
	{"greythr community forum is not a board", "https://community.greythr.com/hire/jobs/x", "", "", "", false},
	{"darwinbox HRMS login is not a careers board", "https://atherenergy.darwinbox.in/user/login", "", "", "", false},
	{"darwinbox marketing site is not a board", "https://www.darwinbox.com/careers/open-roles", "", "", "", false},
	{"phenom custom domain is not derivable", "https://careers.clearchoice.com/us/en/job/P-100864/Lead-Prosthodontist", "", "", "", false},

	// keyedsegment: Workstream's 8-hex employer id after /j/; an employer slug is not the board.
	{"workstream posting", "https://www.workstream.us/j/062be02e/baskin-robbins/los-angeles-21909/crew-member-2-eadf20a0?locale=en&referer_source=Applicant", "workstream", "062be02e", "https://www.workstream.us/j/062be02e", true},
	{"workstream employer root", "https://www.workstream.us/j/965a796b", "workstream", "965a796b", "https://www.workstream.us/j/965a796b", true},
	{"workstream employer slug is not the board", "https://www.workstream.us/j/finecasual", "", "", "", false},
	{"workstream marketing page has no board", "https://www.workstream.us/pricing", "", "", "", false},

	// keyedsegment: LiveHire's tenant segment after /careers/; the segment is lower-case
	// alphanumeric with hyphens. All three URL shapes — board listing, /jobs, and posting —
	// collapse to the board's own listing page.
	{"livehire posting", "https://www.livehire.com/careers/flybuys/job/6HX8X/MSX0L6VHM4/engineering-manager-loyalty-platform-12-month-contract", "livehire", "flybuys", "https://www.livehire.com/careers/flybuys", true},
	{"livehire jobs listing", "https://www.livehire.com/careers/flybuys/jobs", "livehire", "flybuys", "https://www.livehire.com/careers/flybuys", true},
	{"livehire board root", "https://www.livehire.com/careers/flybuys", "livehire", "flybuys", "https://www.livehire.com/careers/flybuys", true},
	{"livehire segment with hyphen", "https://www.livehire.com/careers/phare-consulting-group/apply/b1a7e00000004c008a00000000000028", "livehire", "phare-consulting-group", "https://www.livehire.com/careers/phare-consulting-group", true},
	{"livehire careers index is not a board", "https://www.livehire.com/careers", "", "", "", false},
	{"livehire marketing page is not a board", "https://www.livehire.com/pricing", "", "", "", false},
	{"livehire api path is not a board", "https://www.livehire.com/careers-api/talent-job-search", "", "", "", false},
	{"livehire underscore segment is not a board", "https://www.livehire.com/careers/Flybuys_AU", "", "", "", false},

	// Posting-only links: the board these adapters crawl is not in the URL at all.
	{"gusto posting carries no board", "https://jobs.gusto.com/postings/quality-it-solutions-corp-full-stack-developer-9c8d3a4c-f831-4f68-b552-43124cab8591", "", "", "", false},
	{"schoolspring posting carries no board", "https://www.schoolspring.com/jobdetail?jobId=5954920", "", "", "", false},
	{"edjoin posting carries no board", "https://www.edjoin.org/Home/JobPosting/2285802", "", "", "", false},
	// IdealTraits: the board is the business id the career-site path carries base64-encoded
	// (MTE2MjI5Mg== is 1162292). The posting links are real ones; they name the employer's
	// slug but not its id, so they carry no board.
	{"idealtraits career site", "https://app.idealtraits.com/Comma-Insurance/MTE2MjI5Mg==/careers", "idealtraits", "1162292", "https://app.idealtraits.com/Comma-Insurance/MTE2MjI5Mg==/careers", true},
	{"idealtraits career site page", "https://app.idealtraits.com/Williams-Insurance-Group/MTAxMTA=/careers?page=2", "idealtraits", "10110", "https://app.idealtraits.com/Williams-Insurance-Group/MTAxMTA=/careers", true},
	{"idealtraits career site with escaped slug", "https://app.idealtraits.com/Goodwin,-Lademan-&amp;-Associates,-Inc./MTE2MzQ2OQ==/careers", "idealtraits", "1163469", "https://app.idealtraits.com/Goodwin,-Lademan-&amp;-Associates,-Inc./MTE2MzQ2OQ==/careers", true},
	{"idealtraits posting carries no board", "https://app.idealtraits.com/career/Comma-Insurance/367535CPG", "", "", "", false},
	{"idealtraits posting with ampersand slug carries no board", "https://app.idealtraits.com/career/Cragin-&-Pike/369329CPG", "", "", "", false},
	{"idealtraits non-numeric key is not a board", "https://app.idealtraits.com/Acme/YWJj/careers", "", "", "", false},
	{"idealtraits key that is not base64", "https://app.idealtraits.com/Acme/not*base64/careers", "", "", "", false},
	{"idealtraits bare host", "https://app.idealtraits.com/", "", "", "", false},
	// HigherMe: only the brand career page names a board; a posting link (/jobs/<id>) names a
	// posting id, which is not a brand and must not be read as one.
	{"higherme brand career page", "https://app.higherme.com/careers/6078977d8dfec", "higherme", "6078977d8dfec", "https://app.higherme.com/careers/6078977d8dfec", true},
	{"higherme career page with query", "https://app.higherme.com/careers/6243d6f880266?utm_source=x#top", "higherme", "6243d6f880266", "https://app.higherme.com/careers/6243d6f880266", true},
	{"higherme posting link names no brand", "https://app.higherme.com/jobs/607aff1be9251", "", "", "", false},
	{"higherme bare careers has no board", "https://app.higherme.com/careers", "", "", "", false},
	{"higherme sign-in carries no board", "https://app.higherme.com/login", "", "", "", false},

	// Real posting and board URLs sampled from the catalogue (2026-10-20); each board is the
	// one the provider's boards row stores.
	// gupy: the career-page subdomain.
	{"gupy posting", "https://clicksign.gupy.io/job/eyJqb2JJZCI6MTE0MTU0NzksInNvdXJjZSI6Imd1cHlfcG9ydGFsIn0=?jobBoardSource=gupy_portal", "gupy", "clicksign", "https://clicksign.gupy.io", true},
	{"gupy hyphenated subdomain", "https://confidential-careers.gupy.io/job/eyJqb2JJZCI6MTE0Mzc5NzEsInNvdXJjZSI6Imd1cHlfcG9ydGFsIn0=", "gupy", "confidential-careers", "https://confidential-careers.gupy.io", true},
	{"gupy job-search portal is not a board", "https://portal.gupy.io/job-search/term=dev", "", "", "", false},
	{"gupy marketing site is not a board", "https://www.gupy.io/", "", "", "", false},
	// pandape: the career-site subdomain, on any of the three domains that serve it.
	{"pandape vacancy", "https://braspress.pandape.infojobs.com.br/Detail/3861560", "pandape", "braspress", "https://braspress.pandape.infojobs.com.br", true},
	{"pandape on the catho domain", "https://loungerie.pandape.catho.com.br/Detail/3856149", "pandape", "loungerie", "https://loungerie.pandape.catho.com.br", true},
	{"pandape marketing site is not a board", "https://www.pandape.com.br/", "", "", "", false},
	{"pandape confidential bucket is not a board", "https://empregos.pandape.infojobs.com.br/Detail/3561691", "", "", "", false},
	{"pandape sitemap host names no tenant", "https://pandape.infojobs.com.br/pandapeofertas1.xml.gz", "", "", "", false},
	// pandapelatam: Computrabajo's deployment, its own tenant namespace and so its own provider.
	{"pandape latam vacancy", "https://netcall.pandape.computrabajo.com/Detail/13958406?utm_source=Jobgether", "pandapelatam", "netcall", "https://netcall.pandape.computrabajo.com", true},
	{"pandape latam marketing redirect is not a board", "https://www.pandape.computrabajo.com/", "", "", "", false},
	{"pandape latam confidential bucket is not a board", "https://empleos.pandape.computrabajo.com/Detail/9842801", "", "", "", false},
	// jobscore: the company shortcode behind the platform's "careers" or "jobs" word.
	{"jobscore posting", "https://careers.jobscore.com/careers/ariasystems/jobs/implementation-consultant-a6n7LJEMraPQGI8xheCJ7l?ref=rss&sid=68", "jobscore", "ariasystems", "https://careers.jobscore.com/careers/ariasystems/jobs/implementation-consultant-a6n7LJEMraPQGI8xheCJ7l", true},
	{"jobscore feed", "https://careers.jobscore.com/jobs/ierustechnologiesinc/feed.json?sort=date", "jobscore", "ierustechnologiesinc", "https://careers.jobscore.com/jobs/ierustechnologiesinc/feed.json", true},
	{"jobscore bare careers has no board", "https://careers.jobscore.com/careers", "", "", "", false},
}
