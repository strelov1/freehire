package atsboard

// recognizeCases1 is part 1 of 7 of the TestRecognize table, in order.
var recognizeCases1 = []recognizeCase{
	// path
	{"greenhouse vacancy strips utm", "https://job-boards.greenhouse.io/alpaca/jobs/5745893004?utm=x#top", "greenhouse", "alpaca", "https://job-boards.greenhouse.io/alpaca/jobs/5745893004", true},
	{"greenhouse board listing", "https://job-boards.greenhouse.io/alpaca", "greenhouse", "alpaca", "https://job-boards.greenhouse.io/alpaca", true},
	{"lever strips /apply", "https://jobs.lever.co/offchainlabs/52c01c91/apply", "lever", "offchainlabs", "https://jobs.lever.co/offchainlabs/52c01c91", true},
	{"lever eu data-residency host", "https://jobs.eu.lever.co/coinspaid/244123b5-ffbb/apply?x=1", "lever", "coinspaid", "https://jobs.eu.lever.co/coinspaid/244123b5-ffbb", true},
	{"ashby vacancy", "https://jobs.ashbyhq.com/blitzy/a741b4e8-8799", "ashby", "blitzy", "https://jobs.ashbyhq.com/blitzy/a741b4e8-8799", true},
	{"humanbit vacancy", "https://jobs.humanbit.ai/scrabble-jigsaw/jobs/3e4ec4a3-65f4-41c2-b5d5-a9e12c5fdf4f", "humanbit", "scrabble-jigsaw", "https://jobs.humanbit.ai/scrabble-jigsaw/jobs/3e4ec4a3-65f4-41c2-b5d5-a9e12c5fdf4f", true},
	{"recrutei vacancy", "https://jobs.recrutei.com.br/digisystem/vacancy/157190-desenvolvedor-fullstack-senior", "recrutei", "digisystem", "https://jobs.recrutei.com.br/digisystem/vacancy/157190-desenvolvedor-fullstack-senior", true},
	{"pyjamahr vacancy", "https://jobs.pyjamahr.com/dodo-payments/backend-engineer-rust-4", "pyjamahr", "dodo-payments", "https://jobs.pyjamahr.com/dodo-payments/backend-engineer-rust-4", true},
	{"talenthr vacancy", "https://jobs.talenthr.io/dnext/senior-backend-developer-2/22", "talenthr", "dnext", "https://jobs.talenthr.io/dnext/senior-backend-developer-2/22", true},
	{"deel path", "https://jobs.deel.com/acme/jobs/123", "deel", "acme", "https://jobs.deel.com/acme/jobs/123", true},
	{"jobvite path", "https://jobs.jobvite.com/acme/job/oABC", "jobvite", "acme", "https://jobs.jobvite.com/acme/job/oABC", true},

	// Reserved segments — a platform path word is never a tenant. Jobvite serves the same
	// board bare and behind a "careers" portal segment, so the first segment read the portal
	// word as the board and onboarded nothing; Greenhouse's embed machinery has no board in
	// the path at all (the slug is in the `for=` param, which atsdetect reads).
	{"jobvite portal segment skipped", "https://jobs.jobvite.com/careers/ness/jobs", "jobvite", "ness", "https://jobs.jobvite.com/careers/ness/jobs", true},
	// HERP's board sits behind the platform's own "v1" path word, the same
	// reserved-leading-segment shape Gusto's "/boards/<board>" already uses.
	{"herp job posting", "https://herp.careers/v1/a244/GnoQonoXGBZi", "herp", "a244", "https://herp.careers/v1/a244/GnoQonoXGBZi", true},
	{"herp bare v1 has no board", "https://herp.careers/v1", "", "", "", false},
	// HRMOS's board sits behind the platform's own "pages" path word, the same
	// reserved-leading-segment shape Gusto's "/boards/<board>" already uses.
	{"hrmos job posting", "https://hrmos.co/pages/cyberagent-group/jobs/900100", "hrmos", "cyberagent-group", "https://hrmos.co/pages/cyberagent-group/jobs/900100", true},
	{"hrmos bare pages has no board", "https://hrmos.co/pages", "", "", "", false},
	{"greenhouse embed app has no board", "https://job-boards.greenhouse.io/embed/job_app?token=1", "", "", "", false},
	{"greenhouse embed script has no board", "https://boards.greenhouse.io/embed/job_board/js?for=acme", "", "", "", false},
	// The CDN host leads with the same "job-boards" label the real board hosts do, so only a
	// full-host rule declines it; every Greenhouse job page links it from <head>.
	{"greenhouse CDN asset has no board", "https://job-boards.cdn.greenhouse.io/assets/entry-ZTzpC0b7.css", "", "", "", false},
	{"greenhouse EU board still resolves", "https://job-boards.eu.greenhouse.io/lionhires/jobs/4941013101", "greenhouse", "lionhires", "https://job-boards.eu.greenhouse.io/lionhires/jobs/4941013101", true},

	// Manatal's hosted career-page domain is path-based, not subdomain-based, and its boards
	// live in manatal.yml — careerspage.yml is deliberately empty.
	{"manatal careers-page posting", "https://www.careers-page.com/nearshore-business-solutions/job/5W939XW3", "manatal", "nearshore-business-solutions", "https://www.careers-page.com/nearshore-business-solutions/job/5W939XW3", true},
	{"manatal careers-page listing", "https://www.careers-page.com/hiretidal", "manatal", "hiretidal", "https://www.careers-page.com/hiretidal", true},

	// Gusto: the board is the whole "<company-slug>-<company-uuid>" segment behind the
	// platform's own "boards" word. A /postings/ URL names no board at all — the uuid it ends
	// with is the POSTING's — so it is declined rather than turned into a board no crawl can
	// resolve.
	{"gusto board listing", "https://jobs.gusto.com/boards/100-degrees-consulting-ec8f0e9c-d6b4-4a32-8544-9c213ca6bc90?page=2", "gusto", "100-degrees-consulting-ec8f0e9c-d6b4-4a32-8544-9c213ca6bc90", "https://jobs.gusto.com/boards/100-degrees-consulting-ec8f0e9c-d6b4-4a32-8544-9c213ca6bc90", true},
	{"gusto board listing trailing slash", "https://jobs.gusto.com/boards/121g-consulting-llc-f41b3691-0c93-4406-acb7-6b28c2c69601/", "gusto", "121g-consulting-llc-f41b3691-0c93-4406-acb7-6b28c2c69601", "https://jobs.gusto.com/boards/121g-consulting-llc-f41b3691-0c93-4406-acb7-6b28c2c69601", true},
	{"gusto posting carries the posting uuid, not the board", "https://jobs.gusto.com/postings/grupo-ei-el-paso-warehouse-team-lead-dc55dd36-ff48-49d3-9904-9a2c5746286d", "", "", "", false},
	// AppOne: the portal slug is the board, verbatim (the adapter fetches it as stored). Its
	// posting URLs name no portal at all, and the portal SPA's own machinery shares the host.
	{"appone portal", "https://jobs.appone.com/athomecareagency", "appone", "athomecareagency", "https://jobs.appone.com/athomecareagency", true},
	{"appone portal keeps its case", "https://jobs.appone.com/QCRP?utm_source=x", "appone", "QCRP", "https://jobs.appone.com/QCRP", true},
	{"appone posting url names no portal", "https://apply.appone.com/job/6ac801466826443a3bbf043c", "", "", "", false},
	{"appone portal api is not a board", "https://jobs.appone.com/api/portal/v1/companyjobposts/CRWA", "", "", "", false},
	{"appone logo is not a board", "https://jobs.appone.com/company-images/logos/8882DEC0E9374B97AB9AF455EAAF5A09.png", "", "", "", false},
	{"appone bare host", "https://jobs.appone.com/", "", "", "", false},
	{"gusto bare postings path", "https://jobs.gusto.com/postings", "", "", "", false},
	{"gusto bare boards path", "https://jobs.gusto.com/boards", "", "", "", false},
	{"gusto bare host", "https://jobs.gusto.com", "", "", "", false},

	// pathprefix — the ATS's OWN API host. A careers page on the employer's domain loads its
	// listing over XHR, so the board is named in that API URL and nowhere else in the page.
	{"ashby posting API", "https://api.ashbyhq.com/posting-api/job-board/phantom?includeCompensation=false", "ashby", "phantom", "https://api.ashbyhq.com/posting-api/job-board/phantom", true},
	{"greenhouse boards API", "https://boards-api.greenhouse.io/v1/boards/anthropic/jobs", "greenhouse", "anthropic", "https://boards-api.greenhouse.io/v1/boards/anthropic", true},
	{"lever postings API", "https://api.lever.co/v0/postings/matchgroup?mode=json", "lever", "matchgroup", "https://api.lever.co/v0/postings/matchgroup", true},
	{"api host without a board", "https://api.ashbyhq.com/posting-api/job-board", "", "", "", false},
	{"humanbit bare host no board", "https://jobs.humanbit.ai/", "", "", "", false},
	{"recrutei bare host no board", "https://jobs.recrutei.com.br/", "", "", "", false},
	{"pyjamahr bare host no board", "https://jobs.pyjamahr.com/", "", "", "", false},
	{"api host off-prefix path", "https://api.lever.co/v1/something/else", "", "", "", false},
	// jobappnetwork (talentReef): the platform's own public apply link already carries the
	// board in this shape, so this is the same pathprefix mechanism reused on a host that is
	// pasted by a person rather than called only by XHR.
	{"jobappnetwork apply link", "https://apply.jobappnetwork.com/clients/20448/posting/7216553/", "jobappnetwork", "20448", "https://apply.jobappnetwork.com/clients/20448", true},
	{"jobappnetwork host with no client segment", "https://apply.jobappnetwork.com/talentreef", "", "", "", false},

	// SmartRecruiters serves a posting either bare (<company>/<posting>) or behind a portal
	// segment (<portal>/<company>/<posting>). The employer is the segment before the
	// posting, never the first one — reading the first turned a portal slug into a board.
	{"smartrecruiters bare posting", "https://jobs.smartrecruiters.com/BHFT/744000139104759-senior-compliance-officer", "smartrecruiters", "BHFT", "https://jobs.smartrecruiters.com/BHFT/744000139104759-senior-compliance-officer", true},
	{"smartrecruiters posting behind a portal segment", "https://jobs.smartrecruiters.com/ni/BHFT/6fc8fa0d-1447-4887-9bae-945406ca8500-talent-acquisition-manager", "smartrecruiters", "BHFT", "https://jobs.smartrecruiters.com/ni/BHFT/6fc8fa0d-1447-4887-9bae-945406ca8500-talent-acquisition-manager", true},
	{"smartrecruiters board listing", "https://jobs.smartrecruiters.com/BHFT", "smartrecruiters", "BHFT", "https://jobs.smartrecruiters.com/BHFT", true},
	// The Apply button leaves the posting for a one-click form addressed by publication
	// uuid. Its employer is named in the path (/company/<board>/), and without this the
	// first segment — "oneclick-ui", the product's own machinery — is read as the board.
	{"smartrecruiters one-click apply form", "https://jobs.smartrecruiters.com/oneclick-ui/company/Blend360/publication/59957d76-615a-4809-a282-bcee1120ca7d?dcr_ci=Blend360", "smartrecruiters", "Blend360", "https://jobs.smartrecruiters.com/Blend360", true},

	// pathpair — werecruit: board = the first two path segments verbatim, "<locale>/<tenant>".
	// Unlike pathlocalepair/pathlocale, the locale here is REQUIRED and kept, never folded: a
	// tenant not configured for a locale answers it with an empty listing.
	{"werecruit posting", "https://careers.werecruit.io/en/idiap/offers/postdoctoral-researcher-49895d", "werecruit", "en/idiap", "https://careers.werecruit.io/en/idiap", true},
	{"werecruit listing", "https://careers.werecruit.io/fr/axiom-services", "werecruit", "fr/axiom-services", "https://careers.werecruit.io/fr/axiom-services", true},
	{"werecruit only a locale segment", "https://careers.werecruit.io/en", "", "", "", false},
	{"werecruit bare host", "https://careers.werecruit.io/", "", "", "", false},
	{"werecruit redirect page is not a tenant", "https://careers.werecruit.io/en/redirectToPage", "", "", "", false},
	{"werecruit redirect page with a query", "https://careers.werecruit.io/fr/redirecttopage?to=offers", "", "", "", false},

	// pathlocale — Rippling: an optional leading xx-XX locale segment is skipped; canonical
	// collapses to the board root so a locale-prefixed vacancy, a bare vacancy, and the
	// listing all map to one board.
	{"rippling locale-prefixed vacancy", "https://ats.rippling.com/en-GB/satomic/jobs/48384892-1b6b?utm=x", "rippling", "satomic", "https://ats.rippling.com/satomic", true},
	{"rippling no locale vacancy", "https://ats.rippling.com/satomic/jobs/34aaf2aa", "rippling", "satomic", "https://ats.rippling.com/satomic", true},
	{"rippling board listing", "https://ats.rippling.com/satomic", "rippling", "satomic", "https://ats.rippling.com/satomic", true},
	{"rippling locale only no tenant", "https://ats.rippling.com/en-GB", "", "", "", false},
	// "/jobs/<uuid>" names no tenant: it 308s to /rippling/jobs/<uuid> for ANY uuid, real or not.
	{"rippling tenantless posting", "https://ats.rippling.com/jobs/379fed0e-d42d-4e77-a4ae-05f11ecb2281", "", "", "", false},
	{"rippling locale tenantless posting", "https://ats.rippling.com/en-US/jobs/379fed0e-d42d-4e77-a4ae-05f11ecb2281", "", "", "", false},

	// pathlocalepair — Dayforce: every career site is on one host under
	// "<culture?>/<tenant>/<site>" and the board is the two segments after the culture. The
	// culture is dropped: a posting keeps one id across the cultures it is translated into,
	// which is why the ingest adapter folds it off the board too. Tenant and site are
	// case-insensitive at the platform, so the board is folded to the lower case the board
	// file holds.
	{"dayforce locale-prefixed vacancy", "https://jobs.dayforcehcm.com/en-US/dcrusa/join-us/jobs/12345?utm=x#top", "dayforce", "dcrusa/join-us", "https://jobs.dayforcehcm.com/dcrusa/join-us", true},
	{"dayforce vacancy with no locale", "https://jobs.dayforcehcm.com/dcrusa/join-us/jobs/12345", "dayforce", "dcrusa/join-us", "https://jobs.dayforcehcm.com/dcrusa/join-us", true},
	{"dayforce site listing", "https://jobs.dayforcehcm.com/en-US/4refuel/candidateportal", "dayforce", "4refuel/candidateportal", "https://jobs.dayforcehcm.com/4refuel/candidateportal", true},
	{"dayforce site listing trailing slash", "https://jobs.dayforcehcm.com/en-US/4refuel/candidateportal/", "dayforce", "4refuel/candidateportal", "https://jobs.dayforcehcm.com/4refuel/candidateportal", true},
	{"dayforce mixed-case board folds", "https://jobs.dayforcehcm.com/en-US/DCRUSA/Join-Us/jobs/1", "dayforce", "dcrusa/join-us", "https://jobs.dayforcehcm.com/dcrusa/join-us", true},
	// A site that publishes in no English culture is entered in dayforce.yml WITH its culture
	// ("agfgroup/employeereferralposting/fr-CA"), so this board is a prefix of the stored one
	// rather than equal to it — the contribution flow will see such a link as a new board.
	// Keeping the culture instead would break the other 3,029 entries, whose links carry
	// en-US and whose stored board has no culture segment at all.
	{"dayforce non-english culture stays out of the board", "https://jobs.dayforcehcm.com/fr-CA/agfgroup/employeereferralposting", "dayforce", "agfgroup/employeereferralposting", "https://jobs.dayforcehcm.com/agfgroup/employeereferralposting", true},
	{"dayforce tenant with no site", "https://jobs.dayforcehcm.com/en-US/dcrusa", "", "", "", false},
	{"dayforce locale only", "https://jobs.dayforcehcm.com/en-US", "", "", "", false},
	{"dayforce bare host", "https://jobs.dayforcehcm.com", "", "", "", false},
	// The platform's own machinery shares the tenants' host, and both shapes are linked from
	// every page on it — read as career sites they name the boards "_next/static" and "api/geo".
	{"dayforce app bundle is not a board", "https://jobs.dayforcehcm.com/_next/static/chunks/main.js", "", "", "", false},
	{"dayforce listing api is not a board", "https://jobs.dayforcehcm.com/api/geo/dcrusa/jobposting/search", "", "", "", false},
}
