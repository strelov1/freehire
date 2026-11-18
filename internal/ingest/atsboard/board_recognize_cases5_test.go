package atsboard

// recognizeCases5 is part 5 of 7 of the TestRecognize table, in order.
var recognizeCases5 = []recognizeCase{
	// Workday's public URL may prefix the site with an xx-XX locale the CXS API omits — skip it
	{"workday locale-prefixed site", "https://gm.wd5.myworkdayjobs.com/en-US/Careers_GM/job/x/Eng_JR-1", "workday", "gm.wd5.myworkdayjobs.com/Careers_GM", "https://gm.wd5.myworkdayjobs.com/Careers_GM", true},
	{"workday bare-language posting", "https://repsol.wd3.myworkdayjobs.com/es/Repsol/job/Madrid/Tecnico_R123", "workday", "repsol.wd3.myworkdayjobs.com/Repsol", "https://repsol.wd3.myworkdayjobs.com/Repsol", true},
	{"workday two-letter site posting", "https://acme.wd1.myworkdayjobs.com/pt/job/Lisboa/Eng_R1", "workday", "acme.wd1.myworkdayjobs.com/pt", "https://acme.wd1.myworkdayjobs.com/pt", true},
	{"workday two-letter site listing", "https://acme.wd1.myworkdayjobs.com/pt", "workday", "acme.wd1.myworkdayjobs.com/pt", "https://acme.wd1.myworkdayjobs.com/pt", true},

	// CatsOne's adapter fetches "https://<board>/careers", so the board is the WHOLE host —
	// the same shape catsone.yml stores. Reading the leftmost label instead handed back
	// "authoritypartnersinc", which no crawl can resolve.
	{"catsone vacancy", "https://authoritypartnersinc.catsone.com/careers/12345-senior-engineer", "catsone", "authoritypartnersinc.catsone.com", "https://authoritypartnersinc.catsone.com", true},
	{"catsone board listing", "https://bfc.catsone.com/careers", "catsone", "bfc.catsone.com", "https://bfc.catsone.com", true},

	// Avature's board is the career-site host, exactly as avature.yml stores it
	// (deloittecm.avature.net). Its tenants on the platform's own domain are derivable;
	// the vanity ones (jobs.ea.com) stay out, like every other custom-domain ATS here.
	{"avature vacancy", "https://koch.avature.net/en_us/careers/jobdetail/sr-engineer/186706", "avature", "koch.avature.net", "https://koch.avature.net", true},
	{"avature board listing", "https://deloittecm.avature.net/careers", "avature", "deloittecm.avature.net", "https://deloittecm.avature.net", true},

	// Umantis: the board is the whole careers host. The vendor-domain tenant is derivable from
	// a URL; a customer vanity host is not, and neither is the vendor's bare apex.
	{"umantis vendor tenant", "https://recruitingapp-2840.umantis.com/Vacancies/2939/Description/1", "umantis", "recruitingapp-2840.umantis.com", "https://recruitingapp-2840.umantis.com", true},
	{"umantis listing", "https://recruitingapp-3031.umantis.com/Jobs/All", "umantis", "recruitingapp-3031.umantis.com", "https://recruitingapp-3031.umantis.com", true},
	{"umantis vanity host unrecognised", "https://jobs.bobst.com/Jobs/All", "", "", "", false},

	// HiringThing is white-labelled across ~25 reseller domains, so the board is the whole
	// careers host: a slug alone does not say which domain answers for it, and the same slug
	// can exist under two resellers. Only the domains hiringthing.yml is actually on are
	// listed; an unlisted one is unrecognised, never a false board.
	{"recruiter.co.kr tenant host", "https://cosmax.recruiter.co.kr/career/jobs/131793", "recruiter_co_kr", "cosmax.recruiter.co.kr", "https://cosmax.recruiter.co.kr", true},
	{"recruiter.co.kr API host is not a tenant", "https://api-recruiter.recruiter.co.kr/position/v1/jobflex", "", "", "", false},
	{"hiringthing vendor domain", "https://4mindsai.hiringthing.com/job/12345/senior-engineer", "hiringthing", "4mindsai.hiringthing.com", "https://4mindsai.hiringthing.com", true},
	{"hiringthing reseller domain", "https://10federal-partners-inc.prismhr-hire.com/job/98765/technician", "hiringthing", "10federal-partners-inc.prismhr-hire.com", "https://10federal-partners-inc.prismhr-hire.com", true},
	{"hiringthing second reseller domain", "https://1440-by-the-bay.oasisrecruit.com/", "hiringthing", "1440-by-the-bay.oasisrecruit.com", "https://1440-by-the-bay.oasisrecruit.com", true},
	// The BOARD folds to lower case (DNS is case-insensitive, and the board file holds the
	// lower-case spelling); the canonical keeps the link's own, as it does in every host mode.
	{"hiringthing upper-case host folds", "https://A-M-Crawford.HiringThing.com/job/1", "hiringthing", "a-m-crawford.hiringthing.com", "https://A-M-Crawford.HiringThing.com", true},
	{"hiringthing bare reseller apex is not a board", "https://prismhr-hire.com/", "", "", "", false},
	{"hiringthing platform app host is not a tenant", "https://app.hiringthing.com/", "", "", "", false},

	// host+tenant+board mode — UKG: the board is "<host>/<tenant>/<guid>", the three parts
	// the adapter needs to reach LoadSearchResults. The old rule took the first path segment
	// alone, which the adapter rejects outright ("board must be <host>/<tenant>/<guid>"), and
	// pinned one of the four host families. Codes are folded to lower case: UKG answers either
	// spelling (verified live) and the catalogue holds them lower-case, so preserving a link's
	// upper-case spelling would file a board we already crawl as a new one.
	{"ukg vacancy", "https://recruiting.ultipro.com/AAL1000AIPSA/JobBoard/90e3e14e-26e3-46c0-affc-329a34699e20/OpportunityDetail?opportunityId=5d5", "ukg", "recruiting.ultipro.com/aal1000aipsa/90e3e14e-26e3-46c0-affc-329a34699e20", "https://recruiting.ultipro.com/AAL1000AIPSA/JobBoard/90e3e14e-26e3-46c0-affc-329a34699e20", true},
	{"ukg board listing", "https://recruiting.ultipro.com/aam1000aam/JobBoard/c5a88c41-a6d1-4e5d-bf94-4d0432a0df30", "ukg", "recruiting.ultipro.com/aam1000aam/c5a88c41-a6d1-4e5d-bf94-4d0432a0df30", "https://recruiting.ultipro.com/aam1000aam/JobBoard/c5a88c41-a6d1-4e5d-bf94-4d0432a0df30", true},
	{"ukg second us pod", "https://recruiting2.ultipro.com/abc1000abc/JobBoard/11111111-2222-3333-4444-555555555555/OpportunityDetail", "ukg", "recruiting2.ultipro.com/abc1000abc/11111111-2222-3333-4444-555555555555", "https://recruiting2.ultipro.com/abc1000abc/JobBoard/11111111-2222-3333-4444-555555555555", true},
	{"ukg canadian residency host", "https://recruiting.ultipro.ca/CAN5000/JobBoard/1f3e7f92-2b60-4b8d-a893-c948a630e8a8", "ukg", "recruiting.ultipro.ca/can5000/1f3e7f92-2b60-4b8d-a893-c948a630e8a8", "https://recruiting.ultipro.ca/CAN5000/JobBoard/1f3e7f92-2b60-4b8d-a893-c948a630e8a8", true},
	{"ukg per-tenant rec.pro host", "https://accessiblespace.rec.pro.ukg.net/acc1507asei/JobBoard/1f3e7f92-2b60-4b8d-a893-c948a630e8a8/OpportunityDetail?opportunityId=9", "ukg", "accessiblespace.rec.pro.ukg.net/acc1507asei/1f3e7f92-2b60-4b8d-a893-c948a630e8a8", "https://accessiblespace.rec.pro.ukg.net/acc1507asei/JobBoard/1f3e7f92-2b60-4b8d-a893-c948a630e8a8", true},

	// declined
	{"workday bare host no site", "https://generalmotors.wd5.myworkdayjobs.com", "", "", "", false},
	// A UKG URL that names no board: the tenant alone is not enough for the adapter, so a
	// tenant-only link must decline rather than hand back a board id the crawl will reject.
	{"ukg tenant without a board guid", "https://recruiting.ultipro.com/AAL1000AIPSA", "", "", "", false},
	{"ukg bare host", "https://recruiting.ultipro.com", "", "", "", false},
	// a URL carrying only a locale has no derivable site — unrecognized, not a false "en-US" board
	{"workday locale only no site", "https://salesforce.wd12.myworkdayjobs.com/en-US", "", "", "", false},
	// Both halves of these were wrong before atsdetect.FromURL was folded into this table:
	// the lowercase locale read as a career site, and a per-job path with no site read as
	// the site "job". Each produced a board that does not exist — which the contribution
	// flow records as new and pays for, the exact failure this package's doc warns about.
	{"workday lowercase locale is still a locale", "https://trumpf.wd3.myworkdayjobs.com/en-us/trumpf_students/job/apodaca/ar_r1", "workday", "trumpf.wd3.myworkdayjobs.com/trumpf_students", "https://trumpf.wd3.myworkdayjobs.com/trumpf_students", true},
	{"workday mixed-case locale is still a locale", "https://wmg.wd1.myworkdayjobs.com/de-De/wmgglobal/job/berlin/artist_jr1", "workday", "wmg.wd1.myworkdayjobs.com/wmgglobal", "https://wmg.wd1.myworkdayjobs.com/wmgglobal", true},
	{"workday per-job path with no site", "https://acme.wd1.myworkdayjobs.com/job/Berlin/Engineer_R-1", "", "", "", false},
	{"workday details path with no site", "https://acme.wd1.myworkdayjobs.com/details/Engineer_R-1", "", "", "", false},
	{"workday locale then per-job with no site", "https://acme.wd1.myworkdayjobs.com/en-US/job/Berlin/Engineer_R-1", "", "", "", false},

	// Three hosts this table matched but had no test for, each of which named a board that
	// does not exist. atsdetect.FromURL already declined all three; folding it in surfaced
	// them. A false board is the expensive direction — the contribution flow records it as
	// new and pays for it.
	{"workable per-job shortlink carries no board", "https://apply.workable.com/j/EF5014296F/apply", "", "", "", false},
	{"workable company board still resolves", "https://apply.workable.com/acme/j/EF5014296F/", "workable", "acme", "https://apply.workable.com/acme/j/EF5014296F", true},
	{"pageup non-numeric segment is not an institution", "https://careers.pageuppeople.com/cw/en/search", "", "", "", false},
	{"pageup numeric institution id", "https://careers.pageuppeople.com/513/en/job/12345", "pageup", "513", "https://careers.pageuppeople.com/513/en/job/12345", true},
	{"cornerstone regional host has no single-label tenant", "https://uk-ext.eu.csod.com/ux/ats/careersite/1/home", "", "", "", false},
	{"cornerstone plain tenant still resolves", "https://acme.csod.com/ux/ats/careersite/4/home", "cornerstone", "acme", "https://acme.csod.com", true},
	{"ashby bare host no board", "https://jobs.ashbyhq.com", "", "", "", false},
	{"recruitee bare apex no tenant", "https://recruitee.com/", "", "", "", false},
	{"personio bare apex no tenant", "https://jobs.personio.com", "", "", "", false},
	{"selfrecruit bare apex no tenant", "https://selfrecruit.ge/", "", "", "", false},
	{"scalis bare apex no tenant", "https://scalis.ai/", "", "", "", false},
	{"hibob bare apex no tenant", "https://careers.hibob.com", "", "", "", false},
	{"single-tenant geekjob", "https://geekjob.ru/vacancy/6a1e", "", "", "", false},
	{"teamtailor custom domain not derivable", "https://careers.arrive.com/jobs/1", "", "", "", false},
	// A Teamtailor career site links to the platform's own app host. In host mode the whole
	// host is the board, so app.teamtailor.com passed as a tenant — and boardresolve, which
	// takes the first recognized ATS URL in a page, recorded it as the employer's board.
	{"teamtailor platform app host not a tenant", "https://app.teamtailor.com/companies/1/jobs", "", "", "", false},
	{"teamtailor platform dashboard host not a tenant", "https://dashboard.teamtailor.com/", "", "", "", false},
	// tt.teamtailor.com is the vendor's own "powered by Teamtailor" tracking/short-link
	// host, carried on the footer of every tenant career site — not a tenant, same as
	// app/dashboard above. Found via a contribution whose page linked it, misread as the
	// employer's own board (2026-08-11).
	{"teamtailor platform tt shortlink host not a tenant", "https://tt.teamtailor.com/", "", "", "", false},
	// The same platform-host guard applies to subdomain mode, not just host mode: Recruitee's
	// own employer app lives at app.recruitee.com and BambooHR's own support center at
	// help.bamboohr.com — both real, public vendor hosts, neither a tenant named "app" or
	// "help". Before this guard was wired into modeSubdomain, both resolved as false boards.
	{"recruitee platform app host not a tenant", "https://app.recruitee.com/", "", "", "", false},
	{"bamboohr platform help host not a tenant", "https://help.bamboohr.com/s/article/x", "", "", "", false},
	{"keka platform app host not a tenant", "https://app.keka.com/", "", "", "", false},
	// "embed" READS like platform machinery and is not: embed.bamboohr.com is the board of a
	// company actually called Embed, serving live postings, and BambooHR's widget is a PATH on
	// each tenant's own host (<board>.bamboohr.com/jobs/embed2.php) rather than a host of its
	// own. Pinned because adding "embed" to platformLabels on that misreading would silently
	// drop a real board — a guard whose name sounds like machinery needs the host checked, not
	// assumed (2026-09-08).
	{"bamboohr tenant named like machinery still resolves", "https://embed.bamboohr.com/careers/605", "bamboohr", "embed", "https://embed.bamboohr.com", true},
	// Greenhouse's AI-screening opt-out form is one shared endpoint, identical for every
	// customer, and its URL sits in the markup of a career page whose employer board may be
	// named nowhere else. Read as a path board it yields "ai_opt_out_request" — the SAME board
	// for two unrelated employers, which is the tell. It carries no board behind it either:
	// the segment after it is the platform's "job_post", so skipping (reservedSegments) would
	// only move the false board one segment along.
	{"greenhouse ai opt-out form carries no board", "http://app4.greenhouse.io/ai_opt_out_request/job_post/6178374004/ai_opt_out", "", "", "", false},
	{"greenhouse ai opt-out form bare", "https://my.greenhouse.io/ai_opt_out_request", "", "", "", false},

	// jobs.dover.com: the public job-board host, same dover source and SAME board spelling
	// as app.dover.com — the board is the first segment alone; the UUID second segment only
	// proves the link names a board (it may be a posting id, so it is not folded in). The
	// canonical collapses to the /<Org> board address, which is the same board an
	// app.dover.com link spells as /jobs/<Org>.
	{"dover job board link", "https://jobs.dover.com/Deepspatial/a5546acf-45c3-44ff-bedd-a4314fd0f37d", "dover", "Deepspatial", "https://jobs.dover.com/Deepspatial", true},
	{"dover job board mixed-case host", "https://JOBS.Dover.COM/Deepspatial/a5546acf-45c3-44ff-bedd-a4314fd0f37d", "dover", "Deepspatial", "https://jobs.dover.com/Deepspatial", true},
	{"dover job board with posting path", "https://jobs.dover.com/Deepspatial/a5546acf-45c3-44ff-bedd-a4314fd0f37d/some-posting-slug", "dover", "Deepspatial", "https://jobs.dover.com/Deepspatial", true},
	{"dover job board bare org is a marketing shell", "https://jobs.dover.com/Deepspatial", "", "", "", false},
	{"dover job board vendor listing carries no tenant", "https://jobs.dover.com/startup-jobs", "", "", "", false},
	{"dover job board non-uuid second segment declined", "https://jobs.dover.com/Deepspatial/not-a-uuid", "", "", "", false},
	{"dover job board extra segments declined", "https://jobs.dover.com/Deepspatial/a5546acf-45c3-44ff-bedd-a4314fd0f37d/slug/extra", "", "", "", false},
	{"dover job board subdomain declined", "https://foo.jobs.dover.com/Deepspatial/a5546acf-45c3-44ff-bedd-a4314fd0f37d", "", "", "", false},
	{"dover job board www host declined", "https://www.jobs.dover.com/Deepspatial/a5546acf-45c3-44ff-bedd-a4314fd0f37d", "", "", "", false},
}
