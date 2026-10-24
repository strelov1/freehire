package atsboard

import "testing"

// TestRecognizeBoard checks the network-free URL→(source, board, canonical) parse across both
// extraction modes: path (board = first path segment on a fixed host) and subdomain (board =
// leftmost DNS label, canonical collapses to the bare host). A single-tenant/unknown host or a
// board-less URL is declined.
func TestRecognize(t *testing.T) {
	cases := []struct {
		name          string
		raw           string
		wantSource    string
		wantBoard     string
		wantCanonical string
		wantOK        bool
	}{
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

		// pathlocale — Rippling: an optional leading xx-XX locale segment is skipped; canonical
		// collapses to the board root so a locale-prefixed vacancy, a bare vacancy, and the
		// listing all map to one board.
		{"rippling locale-prefixed vacancy", "https://ats.rippling.com/en-GB/satomic/jobs/48384892-1b6b?utm=x", "rippling", "satomic", "https://ats.rippling.com/satomic", true},
		{"rippling no locale vacancy", "https://ats.rippling.com/satomic/jobs/34aaf2aa", "rippling", "satomic", "https://ats.rippling.com/satomic", true},
		{"rippling board listing", "https://ats.rippling.com/satomic", "rippling", "satomic", "https://ats.rippling.com/satomic", true},
		{"rippling locale only no tenant", "https://ats.rippling.com/en-GB", "", "", "", false},

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
		// The platform's own paths take a locale prefix too, so the guard has to read the segment
		// the BOARD would start at rather than the URL's first one.
		{"dayforce locale-prefixed app bundle is not a board", "https://jobs.dayforcehcm.com/en-US/_next/data/build/dcrusa.json", "", "", "", false},
		{"dayforce locale-prefixed api is not a board", "https://jobs.dayforcehcm.com/en-US/api/geo/dcrusa/jobposting/search", "", "", "", false},

		// query — Paycor: nothing but the clientId parameter names the board, since the path is
		// the same for every employer on the platform. The canonical collapses to the board's own
		// career home, so a posting and the listing are one board.
		{"paycor posting", "https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=4028f88b24c330a20124eb33b4ad1653&id=8a7883a6&source=&lang=en", "paycor", "4028f88b24c330a20124eb33b4ad1653", "https://recruitingbypaycor.com/career/CareerHome.action?clientId=4028f88b24c330a20124eb33b4ad1653", true},
		{"paycor career home", "https://recruitingbypaycor.com/career/CareerHome.action?clientId=8a29a018478a87e70147a7588a6343cf", "paycor", "8a29a018478a87e70147a7588a6343cf", "https://recruitingbypaycor.com/career/CareerHome.action?clientId=8a29a018478a87e70147a7588a6343cf", true},
		{"paycor upper-case client id folds", "https://www.recruitingbypaycor.com/career/JobIntroduction.action?clientId=8A29A018478A87E70147A7588A6343CF&id=1", "paycor", "8a29a018478a87e70147a7588a6343cf", "https://www.recruitingbypaycor.com/career/CareerHome.action?clientId=8a29a018478a87e70147a7588a6343cf", true},
		{"paycor without a client id", "https://recruitingbypaycor.com/career/CareerHome.action", "", "", "", false},
		{"paycor empty client id", "https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=&id=1", "", "", "", false},
		// A parameter is a weaker signal than a path segment on a board-only host, so the value
		// must have the shape of a client key — the same guard pageup's pathnumeric mode carries.
		{"paycor malformed client id is not a board", "https://recruitingbypaycor.com/career/CareerHome.action?clientId=not-a-board", "", "", "", false},
		{"paycor truncated client id is not a board", "https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=4028f88b24c330a2&id=1", "", "", "", false},
		// HR-Manager: the board is the customer alias, named only by the customer-level pages.
		{"hrmanager listing", "https://candidate.hr-manager.net/vacancies/list.aspx?customer=energinet", "hrmanager", "energinet", "https://candidate.hr-manager.net/vacancies/list.aspx?customer=energinet", true},
		{"hrmanager job agent folds case", "https://candidate.hr-manager.net/Agent/Subscription.aspx?customer=GFforsikring_tr", "hrmanager", "gfforsikring_tr", "https://candidate.hr-manager.net/vacancies/list.aspx?customer=gfforsikring_tr", true},
		{"hrmanager posting names only the cid", "https://candidate.hr-manager.net/ApplicationInit.aspx?cid=316&ProjectId=188414&DepartmentId=21674&MediaId=5", "", "", "", false},
		{"hrmanager numeric customer is not an alias", "https://candidate.hr-manager.net/vacancies/list.aspx?customer=316", "", "", "", false},
		{"hrmanager job-portal API", "https://api.hr-manager.net/JobPortal.svc/kimdemo/PositionList/rss/?protype=RecruitmentProject&incads=true", "hrmanager", "kimdemo", "https://api.hr-manager.net/JobPortal.svc/kimdemo", true},
		{"hrmanager job-portal API lower-case path", "https://api.hr-manager.net/jobportal.svc/energinet/positionlist/json/", "hrmanager", "energinet", "https://api.hr-manager.net/JobPortal.svc/energinet", true},
		{"hrmanager API without a customer", "https://api.hr-manager.net/JobPortal.svc/", "", "", "", false},
		// Tenstreet: the board is the carrier's numeric company_id, named by its profile and
		// by every posting; both collapse to the profile, which is the listing.
		{"tenstreet posting", "https://pulse.tenstreet.com/share_app_show_active_jobs.php?active_job_id=123622&company_id=774&portal_user_id=1", "tenstreet", "774", "https://pulse.tenstreet.com/share_app_index.php?company_id=774", true},
		{"tenstreet carrier profile", "https://pulse.tenstreet.com/share_app_index.php?company_id=2337095", "tenstreet", "2337095", "https://pulse.tenstreet.com/share_app_index.php?company_id=2337095", true},
		{"tenstreet non-numeric company is not a board", "https://pulse.tenstreet.com/share_app_index.php?company_id=acme", "", "", "", false},
		{"tenstreet link without a company", "https://pulse.tenstreet.com/share_app_show_active_jobs.php?active_job_id=123622", "", "", "", false},

		// querypair — ADP Workforce Now: cid names the tenant, ccId the career centre inside it,
		// and the adapter needs both, so the board is the pair. The canonical collapses to the
		// recruitment page carrying only those two.
		{"adp posting", "https://workforcenow.adp.com/mascsr/default/mdf/recruitment/recruitment.html?cid=8a680274-0b2e-46cf-b2cc-49bd2ef0c20f&ccId=9201289910657_2&jobId=123&lang=en_US", "adp", "8a680274-0b2e-46cf-b2cc-49bd2ef0c20f:9201289910657_2", "https://workforcenow.adp.com/mascsr/default/mdf/recruitment/recruitment.html?ccId=9201289910657_2&cid=8a680274-0b2e-46cf-b2cc-49bd2ef0c20f", true},
		{"adp upper-case cid folds", "https://workforcenow.adp.com/mascsr/default/mdf/recruitment/recruitment.html?cid=8A680274-0B2E-46CF-B2CC-49BD2EF0C20F&ccId=19000101_000001", "adp", "8a680274-0b2e-46cf-b2cc-49bd2ef0c20f:19000101_000001", "https://workforcenow.adp.com/mascsr/default/mdf/recruitment/recruitment.html?ccId=19000101_000001&cid=8a680274-0b2e-46cf-b2cc-49bd2ef0c20f", true},
		// Half a pair is not a board: the adapter splits the board on the colon and sends both
		// halves, so a truncated one would 404 every crawl.
		{"adp without a ccId is not a board", "https://workforcenow.adp.com/mascsr/default/mdf/recruitment/recruitment.html?cid=8a680274-0b2e-46cf-b2cc-49bd2ef0c20f", "", "", "", false},
		{"adp without a cid is not a board", "https://workforcenow.adp.com/mascsr/default/mdf/recruitment/recruitment.html?ccId=19000101_000001", "", "", "", false},
		{"adp with neither is not a board", "https://workforcenow.adp.com/mascsr/default/mdf/recruitment/recruitment.html", "", "", "", false},
		{"adp malformed cid is not a board", "https://workforcenow.adp.com/mascsr/default/mdf/recruitment/recruitment.html?cid=acme&ccId=19000101_000001", "", "", "", false},
		{"adp malformed ccId is not a board", "https://workforcenow.adp.com/mascsr/default/mdf/recruitment/recruitment.html?cid=8a680274-0b2e-46cf-b2cc-49bd2ef0c20f&ccId=main", "", "", "", false},
		// Taleo Business Edition: the board is org + career-site number on any pod host; the pod
		// is not part of it. Real apply URLs, harvested 2026-10.
		{"taleobe legacy posting", "https://phf.tbe.taleo.net/phf02/ats/careers/requisition.jsp?org=ADELPHI&cws=43&rid=3413", "taleobe", "ADELPHI:43", "https://phf.tbe.taleo.net/dispatcher/servlet/DispatcherServlet?act=redirectCws&cws=43&org=ADELPHI", true},
		{"taleobe v2 apply link", "https://chj.tbe.taleo.net/chj04/ats/careers/v2/applyRequisition?org=VIBRHEAL&cws=44&rid=20313", "taleobe", "VIBRHEAL:44", "https://chj.tbe.taleo.net/dispatcher/servlet/DispatcherServlet?act=redirectCws&cws=44&org=VIBRHEAL", true},
		{"taleobe tracking parameter", "http://chj.tbe.taleo.net/chj05/ats/careers/requisition.jsp?org=SUN_PHARMA&cws=1&rid=606&source=Indeed", "taleobe", "SUN_PHARMA:1", "http://chj.tbe.taleo.net/dispatcher/servlet/DispatcherServlet?act=redirectCws&cws=1&org=SUN_PHARMA", true},
		{"taleobe lower-case org folds", "https://tre.tbe.taleo.net/tre01/ats/careers/v2/searchResults?org=saintelizabeth&cws=50", "taleobe", "SAINTELIZABETH:50", "https://tre.tbe.taleo.net/dispatcher/servlet/DispatcherServlet?act=redirectCws&cws=50&org=SAINTELIZABETH", true},
		{"taleobe dispatcher link", "https://phe.tbe.taleo.net/dispatcher/servlet/DispatcherServlet?org=ICANN&act=redirectCws&cws=37", "taleobe", "ICANN:37", "https://phe.tbe.taleo.net/dispatcher/servlet/DispatcherServlet?act=redirectCws&cws=37&org=ICANN", true},
		{"taleobe empty cws is not a board", "https://phe.tbe.taleo.net/phe01/ats/careers/v2/jobSearch?act=redirectCws&org=FREEHOUS&cws=", "", "", "", false},
		{"taleobe without an org is not a board", "https://phf.tbe.taleo.net/phf02/ats/careers/requisition.jsp?cws=43&rid=3413", "", "", "", false},
		{"taleo enterprise is a different product", "https://valero.taleo.net/careersection/2/jobdetail.ftl?job=123&org=X&cws=1", "", "", "", false},
		// myjobs.adp.com is a DIFFERENT ADP product with its own adapter: its slug is an
		// adpmyjobs board, never an adp one (whose board is the cid:ccId pair).
		{"adp myjobs career site is adpmyjobs", "https://myjobs.adp.com/stellantisexternalcx/cx/job/5001207766206", "adpmyjobs", "stellantisexternalcx", "https://myjobs.adp.com/stellantisexternalcx/cx/job/5001207766206", true},
		{"adp myjobs job-details link", "https://myjobs.adp.com/1138141confidential/cx/job-details?reqId=5001207191206", "adpmyjobs", "1138141confidential", "https://myjobs.adp.com/1138141confidential/cx/job-details", true},
		// /public/staffing/v1/career-site/<slug> is the platform's own API, which the adapter
		// itself calls; "public" is not a career site.
		{"adp myjobs public API has no board", "https://myjobs.adp.com/public/staffing/v1/career-site/acme", "", "", "", false},
		{"adp myjobs bare host has no board", "https://myjobs.adp.com/", "", "", "", false},

		// AppliTrack serves every district from one host: www.applitrack.com/<board>/onlineapp/….
		{"applitrack posting", "https://www.applitrack.com/aacs/onlineapp/jobpostings/view.asp?AppliTrackJobId=1313", "applitrack", "aacs", "https://www.applitrack.com/aacs/onlineapp/jobpostings/view.asp", true},
		{"applitrack listing", "https://www.applitrack.com/saskatoon/onlineapp/default.aspx", "applitrack", "saskatoon", "https://www.applitrack.com/saskatoon/onlineapp/default.aspx", true},
		{"applitrack bare host has no board", "https://www.applitrack.com/", "", "", "", false},

		// apiBoards — Paylocity: only the listing path names the board. A posting and its apply
		// form carry a posting id and no tenant, so they are declined rather than read as one.
		{"paylocity listing", "https://recruiting.paylocity.com/Recruiting/Jobs/All/7186761b-1318-47f2-b709-d7e7995cd3f3", "paylocity", "7186761b-1318-47f2-b709-d7e7995cd3f3", "https://recruiting.paylocity.com/Recruiting/Jobs/All/7186761b-1318-47f2-b709-d7e7995cd3f3", true},
		{"paylocity posting is not a board", "https://recruiting.paylocity.com/Recruiting/Jobs/Details/4037913", "", "", "", false},
		{"paylocity apply form is not a board", "https://recruiting.paylocity.com/Recruiting/Jobs/Apply/1471799", "", "", "", false},
		{"paylocity bare listing path is not a board", "https://recruiting.paylocity.com/Recruiting/Jobs/All", "", "", "", false},

		// icims — board IS the host (like subdomain/host modes), so the canonical collapses to
		// the bare host: the classic "careers-<slug>.icims.com" host folds to the bare slug
		// (matching icims.yml's usual shape); any other *.icims.com host, including one that
		// merely looks like the classic shape without the "careers-" prefix, is itself the board.
		{"icims classic vacancy", "https://careers-here.icims.com/jobs/81319/lead-agentic-ai-engineer/login", "icims", "here", "https://careers-here.icims.com", true},
		{"icims classic board listing", "https://careers-here.icims.com/jobs/search", "icims", "here", "https://careers-here.icims.com", true},
		{"icims vanity host without the careers- prefix", "https://uscareers-lennox.icims.com/jobs/54590/ux-designer/candidate", "icims", "uscareers-lennox.icims.com", "https://uscareers-lennox.icims.com", true},
		{"icims bare apex is not a board", "https://www.icims.com/", "", "", "", false},
		{"icims host outside icims.com is not recognized", "https://icims.example.com/jobs", "", "", "", false},

		// hosttenant — UKG Ready: the host selects the environment the tenant is hosted in and is
		// not derivable from the tenant id, so the board carries both. The ".careers" suffix is
		// what proves the segment names a career site; the SPA's own REST API lives under the same
		// /ta prefix and names no board.
		{"ukgready vacancy", "https://secure.entertimeonline.com/ta/10284.careers?ShowJob=123456", "ukgready", "secure.entertimeonline.com/10284", "https://secure.entertimeonline.com/ta/10284.careers", true},
		{"ukgready board listing", "https://secure7.saashr.com/ta/6010265.careers", "ukgready", "secure7.saashr.com/6010265", "https://secure7.saashr.com/ta/6010265.careers", true},
		{"ukgready regional host", "https://secure.workforceready.com.au/ta/6162382.careers?ApplyToJob=1", "ukgready", "secure.workforceready.com.au/6162382", "https://secure.workforceready.com.au/ta/6162382.careers", true},
		{"ukgready mykronos host", "https://prd01-hcm01.npr.mykronos.com/ta/6042637.careers", "ukgready", "prd01-hcm01.npr.mykronos.com/6042637", "https://prd01-hcm01.npr.mykronos.com/ta/6042637.careers", true},
		{"ukgready bare apex host", "https://saashr.com/ta/6199997.careers", "ukgready", "saashr.com/6199997", "https://saashr.com/ta/6199997.careers", true},
		{"ukgready upper-case tenant folds", "https://secure3.yourpayrollhr.com/ta/IHR0568.Careers", "ukgready", "secure3.yourpayrollhr.com/ihr0568", "https://secure3.yourpayrollhr.com/ta/ihr0568.careers", true},
		{"ukgready rest api names no board", "https://secure7.saashr.com/ta/rest/ui/recruitment/companies/%7C6010265/job-requisitions", "", "", "", false},
		{"ukgready tenant without the careers suffix", "https://secure7.saashr.com/ta/6010265", "", "", "", false},
		{"ukgready bare host", "https://secure7.saashr.com/", "", "", "", false},

		// hostsite — Paradox: "<instance host>/<site>", lower-cased, canonical the site's job list
		{"paradox posting", "https://olivia.paradox.ai/co/ChickenSaladChick1/Job?job_id=P1-6592465-1", "paradox", "olivia.paradox.ai/chickensaladchick1", "https://olivia.paradox.ai/co/chickensaladchick1/jobs", true},
		{"paradox posting with posting_type", "https://olivia.paradox.ai/co/SonarIncTacoBell/Job?job_id=P1-6357073-1&posting_type=1", "paradox", "olivia.paradox.ai/sonarinctacobell", "https://olivia.paradox.ai/co/sonarinctacobell/jobs", true},
		{"paradox opaque job id", "https://olivia.paradox.ai/co/BurgerKing2/Job?job_id=PDX_BK_1DCB993D-D9FE-47C6-B377-E5CD9AF2E352_21981613", "paradox", "olivia.paradox.ai/burgerking2", "https://olivia.paradox.ai/co/burgerking2/jobs", true},
		{"paradox digit-led site", "https://olivia.paradox.ai/co/2ndFamily28/Job?job_id=PDX_2F_5E4133CD-7D68-4E6A-9966-F03D5B1F555C_24833714", "paradox", "olivia.paradox.ai/2ndfamily28", "https://olivia.paradox.ai/co/2ndfamily28/jobs", true},
		{"paradox eu instance", "https://olivia.eu1.paradox.ai/co/Pandora/Job?job_id=PDX_PA_A22F58C3-7DC3-4E13-90A2-1F54B2381FDE_22070543", "paradox", "olivia.eu1.paradox.ai/pandora", "https://olivia.eu1.paradox.ai/co/pandora/jobs", true},
		{"paradox customer instance", "https://darden.paradox.ai/co/RuthsChris/Job?job_id=PDX_RC_1C7BB604-FFEF-49A3-8480-B1B732C76B30_97829&posting_type=1", "paradox", "darden.paradox.ai/ruthschris", "https://darden.paradox.ai/co/ruthschris/jobs", true},
		{"paradox job list", "https://olivia.paradox.ai/co/KFC28/jobs", "paradox", "olivia.paradox.ai/kfc28", "https://olivia.paradox.ai/co/kfc28/jobs", true},
		{"paradox marketing apex", "https://paradox.ai/co/Acme/Job?job_id=1", "", "", "", false},
		{"paradox path outside /co", "https://olivia.paradox.ai/help/article", "", "", "", false},
		{"paradox /co with no site", "https://olivia.paradox.ai/co/", "", "", "", false},

		// subdomain — canonical collapses to the bare host
		{"recruitee vacancy strips path", "https://acme.recruitee.com/o/senior-go/apply?utm=x", "recruitee", "acme", "https://acme.recruitee.com", true},
		{"recruitee board listing", "https://acme.recruitee.com", "recruitee", "acme", "https://acme.recruitee.com", true},
		{"bamboohr subdomain", "https://acme.bamboohr.com/careers/42", "bamboohr", "acme", "https://acme.bamboohr.com", true},
		{"jobspage vacancy", "https://richads.jobspage.co/9fu6scqy-account-manager", "jobspage", "richads", "https://richads.jobspage.co", true},
		{"jobspage demo board is the vendor's", "https://demo.jobspage.co/", "", "", "", false},
		{"jobspage marketing site is not a tenant", "https://www.jobspage.co/pricing", "", "", "", false},
		{"keka subdomain", "https://100.keka.com/careers", "keka", "100", "https://100.keka.com", true},
		{"personio nested apex subdomain", "https://acme.jobs.personio.com/job/9", "personio", "acme", "https://acme.jobs.personio.com", true},
		{"personio de host", "https://reflex-aerospace-gmbh.jobs.personio.de/job/2679152?display=en#apply", "personio", "reflex-aerospace-gmbh", "https://reflex-aerospace-gmbh.jobs.personio.de", true},
		{"selfrecruit posting strips path", "https://dressup.selfrecruit.ge/a7cdcc00-1c9c-464c-8960-945af0c0e0a4", "selfrecruit", "dressup", "https://dressup.selfrecruit.ge", true},
		{"scalis posting strips path", "https://boldbusiness.scalis.ai/job/5f78d010-b21f-4d22-b037-e86865530f62", "scalis", "boldbusiness", "https://boldbusiness.scalis.ai", true},
		{"softgarden subdomain", "https://moll.softgarden.io/job/123/apply", "softgarden", "moll", "https://moll.softgarden.io", true},
		// softgarden also serves tenants under a regional career host, <tenant>.career.softgarden.de.
		// The tenant label is the same board the adapter fetches at <board>.softgarden.io (verified
		// live: agilitaschweiz answers on both), so only the apex differs. Keyed on softgarden.io
		// alone, the .de host left the label behind a second DNS label and was declined.
		{"softgarden regional career host", "https://agilitaschweiz.career.softgarden.de/jobs/65679426/sap-consultant/", "softgarden", "agilitaschweiz", "https://agilitaschweiz.career.softgarden.de", true},
		// Emply: <tenant>.career.emply.com, the board the adapter fetches at that host. Ad and
		// apply links both name it.
		{"emply ad", "https://molslinjen.career.emply.com/ad/it-supporter-soges-til-molslinjen-i-ronne/12us1b/da", "emply", "molslinjen", "https://molslinjen.career.emply.com", true},
		{"emply apply link", "https://oxfam-dk.career.emply.com/apply/paid-content-producer-til-oxfam-danmark/8xnpso#jobdk", "emply", "oxfam-dk", "https://oxfam-dk.career.emply.com", true},
		// HCS Hiring (symplr): <tenant>.hcshiring.com/jobs/<id>, the board the adapter reads the
		// tenant's API at. The vendor's other hosts are other products and stay unrecognised.
		{"hcshiring posting", "https://capricommunities.hcshiring.com/jobs/TugN1ntScEij8OCB6I_Ezw", "hcshiring", "capricommunities", "https://capricommunities.hcshiring.com", true},
		{"hcshiring posting id with leading underscore", "https://riverviewlodgeal.hcshiring.com/jobs/_YMF6czAe0yKvrkSlh0NqA", "hcshiring", "riverviewlodgeal", "https://riverviewlodgeal.hcshiring.com", true},
		{"hcshiring portal is another product", "https://valleywise.hctsportals.com/jobs/2237498", "", "", "", false},
		{"hcshiring position manager is another product", "https://pm.healthcaresource.com/CS/rrh/#/job/2474", "", "", "", false},
		{"hibob careers subdomain", "https://qogita.careers.hibob.com/jobs/ceb6c947-c906-44d1-a56b-bb33ae5599fa", "hibob", "qogita", "https://qogita.careers.hibob.com", true},
		{"hibob apply tail collapses to the same board", "https://unique.careers.hibob.com/jobs/f8d9a0bc/apply", "hibob", "unique", "https://unique.careers.hibob.com", true},
		// HRM Direct: the board is the tenant subdomain. Its APPLICATION forms are served from the
		// platform's own apply.hrmdirect.com, which without the platform-label guard reads as a
		// company called "apply" — and that host is linked from every posting on the platform.
		{"hrmdirect posting", "https://123loadboard.hrmdirect.com/employment/job-opening.php?req=3456&req_loc=1029#apply", "hrmdirect", "123loadboard", "https://123loadboard.hrmdirect.com", true},
		{"hrmdirect listing", "https://1aautoinc.hrmdirect.com/employment/job-openings.php?search=true", "hrmdirect", "1aautoinc", "https://1aautoinc.hrmdirect.com", true},
		{"hrmdirect listing trailing slash", "https://4air.hrmdirect.com/employment/", "hrmdirect", "4air", "https://4air.hrmdirect.com", true},
		{"hrmdirect upper-case host folds", "https://500DegreesStudio.HRMDirect.com/employment/job-opening.php?req=1", "hrmdirect", "500degreesstudio", "https://500DegreesStudio.HRMDirect.com", true},
		{"hrmdirect apply host is the platform's, not a board", "https://apply.hrmdirect.com/resumes/click2apply.php?req=1", "", "", "", false},
		{"hrmdirect bare apex no tenant", "https://hrmdirect.com/", "", "", "", false},
		// ApplicantStack: the board is the tenant subdomain, folded to lower case — the same
		// tenant is linked as both "SmartAutoCare" and "smartautocare".
		{"applicantstack posting", "https://ozarkhealth.applicantstack.com/x/detail/a23rqqe2fctl", "applicantstack", "ozarkhealth", "https://ozarkhealth.applicantstack.com", true},
		{"applicantstack listing", "https://ama-consult.applicantstack.com/x/openings", "applicantstack", "ama-consult", "https://ama-consult.applicantstack.com", true},
		{"applicantstack upper-case tenant folds", "https://SmartAutoCare.applicantstack.com/x/detail/a2g5b5u0kq8i", "applicantstack", "smartautocare", "https://SmartAutoCare.applicantstack.com", true},
		{"applicantstack marketing host is not a tenant", "https://www.applicantstack.com/job-not-found/", "", "", "", false},
		{"applicantstack asset host is not a tenant", "https://public.applicantstack.com/userdata/gis/logo.jpg", "", "", "", false},
		// HireClick: the board is the tenant subdomain; the vendor's marketing hosts are not.
		{"hireclick posting", "https://senca.hireclick.com/jb/family-development-associate-nebraska-city/view/264587", "hireclick", "senca", "https://senca.hireclick.com", true},
		{"hireclick job board", "https://williamsnd.hireclick.com/jobboard/jobboard.aspx", "hireclick", "williamsnd", "https://williamsnd.hireclick.com", true},
		{"hireclick marketing host is not a tenant", "https://employers.hireclick.com/", "", "", "", false},
		{"hireclick www host is not a tenant", "https://www.hireclick.com/", "", "", "", false},
		{"potok vacancy", "https://oteko.potok.io/open/jobs/1431267", "potok", "oteko", "https://oteko.potok.io", true},
		{"potok listing", "https://domrf.potok.io/open/jobs/", "potok", "domrf", "https://domrf.potok.io", true},
		{"potok platform host is not a tenant", "https://app.potok.io/open/jobs/1", "", "", "", false},
		{"potok infrastructure host is not a tenant", "https://stages.potok.io/", "", "", "", false},
		{"hiringroom vacancy", "https://tatachile.hiringroom.com/jobs/get_vacancy/6acfeaf91c677c42c9135f1a", "hiringroom", "tatachile", "https://tatachile.hiringroom.com", true},
		{"hiringroom marketing host is not a tenant", "https://www.hiringroom.com/jobs", "", "", "", false},
		{"hiringroom apply host names no tenant", "https://hiringroom.com/jobs/get_vacancy/6acfeaf91c677c42c9135f1a/candidates/new", "", "", "", false},
		{"friendwork vacancy", "https://jobs.friend.work/TeDo/97846", "friendwork", "TeDo", "https://jobs.friend.work/TeDo/97846", true},
		// "apply" is HRM Direct's own host and a REAL recruitee board (recruitee.yml tracks it),
		// so the guard is scoped to the apex that needs it. Declining it everywhere would drop a
		// board the catalogue already crawls.
		{"recruitee apply is a tenant, not a platform host", "https://apply.recruitee.com/o/senior-go", "recruitee", "apply", "https://apply.recruitee.com", true},

		// subdomainchain — Huntflow nests its international tenants under a "global" label, and
		// the adapter fetches <board>.huntflow.io, so the board must carry that label too;
		// the leftmost label alone ("thefjx") is a host that 404s.
		{"huntflow regional tenant keeps the region label", "https://thefjx.global.huntflow.io/vacancy/rust-developer-2", "huntflow", "thefjx.global", "https://thefjx.global.huntflow.io", true},
		{"huntflow plain tenant", "https://flowwow.huntflow.io/vacancy/123", "huntflow", "flowwow", "https://flowwow.huntflow.io", true},
		{"huntflow bare apex no tenant", "https://huntflow.io/", "", "", "", false},

		// host mode — board is the whole careers host, regional TLD varies
		{"zoho eu vacancy strips encoded path + query", "https://be-exec.zohorecruit.eu/jobs/Careers/73534000009044079/%D0%9F%D1%80%D0%BE?source=CareerSite", "zohorecruit", "be-exec.zohorecruit.eu", "https://be-exec.zohorecruit.eu", true},
		{"zoho com host", "https://kaptiva.zohorecruit.com/jobs/Careers/568", "zohorecruit", "kaptiva.zohorecruit.com", "https://kaptiva.zohorecruit.com", true},
		{"zoho in host", "https://incubyte.zohorecruit.in/jobs/Careers/141", "zohorecruit", "incubyte.zohorecruit.in", "https://incubyte.zohorecruit.in", true},
		{"zoho bare apex not a board", "https://zohorecruit.com/", "", "", "", false},
		{"jazzhr applytojob", "https://acme.applytojob.com/apply/abc", "jazzhr", "acme", "https://acme.applytojob.com", true},
		{"gethired career site", "https://kidscancercarefoundationofalbe.easyapply.co/", "gethired", "kidscancercarefoundationofalbe", "https://kidscancercarefoundationofalbe.easyapply.co", true},
		{"gethired tenant posting", "https://odysseyhouselouisianainc.easyapply.co/job/behavioral-health-technician", "gethired", "odysseyhouselouisianainc", "https://odysseyhouselouisianainc.easyapply.co", true},
		{"gethired apply link names no tenant", "https://easyapply.co/a/b3be0e78-b890-4ad5-a3de-ac1f7258a0e3", "", "", "", false},
		{"gethired www is not a tenant", "https://www.easyapply.co/", "", "", "", false},
		{"trakstar nested apex", "https://acme.hire.trakstar.com/x", "trakstar", "acme", "https://acme.hire.trakstar.com", true},
		{"teamtailor host", "https://bryter.teamtailor.com/jobs/12345-senior-go", "teamtailor", "bryter.teamtailor.com", "https://bryter.teamtailor.com", true},
		{"factorial host it", "https://muffin.factorial.it/job/1", "factorial", "muffin.factorial.it", "https://muffin.factorial.it", true},
		{"factorialhr base-domain variant", "https://9net.factorialhr.com.br/job/2", "factorial", "9net.factorialhr.com.br", "https://9net.factorialhr.com.br", true},
		// gr8people: one vendor served under two marketing domains, confirmed live (identical
		// frontend, identical GraphQL schema) — the same one-adapter-two-domains shape as Factorial.
		{"gr8people host", "https://etrade.gr8people.com/jobs/4709", "gr8people", "etrade.gr8people.com", "https://etrade.gr8people.com", true},
		{"workgr8 base-domain variant", "https://batesville.workgr8.com/jobs/1146", "gr8people", "batesville.workgr8.com", "https://batesville.workgr8.com", true},
		{"gr8people bare apex not a board", "https://gr8people.com/", "", "", "", false},
		{"eploy tenant host", "https://peigroupweb.eploy.net/vacancies/442/senior-reporter--private-equity.html", "eploy", "peigroupweb.eploy.net", "https://peigroupweb.eploy.net", true},
		{"eploy marketing site is not a tenant", "https://www.eploy.com/customers/", "", "", "", false},
		{"workgr8 bare apex not a board", "https://workgr8.com/", "", "", "", false},

		// host+path mode — Workday: board is "<host>/<site>" (site case preserved)
		{"workday vacancy", "https://generalmotors.wd5.myworkdayjobs.com/Careers_GM/job/Austin/Senior-Software-Engineer_JR-202614238", "workday", "generalmotors.wd5.myworkdayjobs.com/Careers_GM", "https://generalmotors.wd5.myworkdayjobs.com/Careers_GM", true},
		{"workday board listing", "https://generalmotors.wd5.myworkdayjobs.com/Careers_GM", "workday", "generalmotors.wd5.myworkdayjobs.com/Careers_GM", "https://generalmotors.wd5.myworkdayjobs.com/Careers_GM", true},
		{"workday other pod", "https://goodyear.wd1.myworkdayjobs.com/goodyearcareers/job/x", "workday", "goodyear.wd1.myworkdayjobs.com/goodyearcareers", "https://goodyear.wd1.myworkdayjobs.com/goodyearcareers", true},
		// Workday's public URL may prefix the site with an xx-XX locale the CXS API omits — skip it
		{"workday locale-prefixed site", "https://gm.wd5.myworkdayjobs.com/en-US/Careers_GM/job/x/Eng_JR-1", "workday", "gm.wd5.myworkdayjobs.com/Careers_GM", "https://gm.wd5.myworkdayjobs.com/Careers_GM", true},

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

		// HiringThing is white-labelled across ~25 reseller domains, so the board is the whole
		// careers host: a slug alone does not say which domain answers for it, and the same slug
		// can exist under two resellers. Only the domains hiringthing.yml is actually on are
		// listed; an unlisted one is unrecognised, never a false board.
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
		{"successfactors legacy career section is not derivable", "https://career55.sapsf.eu/career?career_ns=job_listing&company=pressglass&career_job_req_id=1395&lang=pl_PL", "", "", "", false},
		{"phenom phenompro posting", "https://qualtrics.phenompro.com/us/en/job/8274275/State-and-Local-Government-Sales-Manager", "phenom", "qualtrics.phenompro.com", "https://qualtrics.phenompro.com", true},
		{"phenom phenompeople.net posting", "https://acadiamigration.phenompeople.net/us/en/job/110212/Substance-Use-Counselor-TCADC-CADC", "phenom", "acadiamigration.phenompeople.net", "https://acadiamigration.phenompeople.net", true},
		{"phenom phenompeople.com posting without a locale", "https://lifetime.phenompeople.com/job/R-174914", "phenom", "lifetime.phenompeople.com", "https://lifetime.phenompeople.com", true},
		// Phenom serves its CDN from the same domains; a career page links there on every load.
		{"phenom cdn asset is not a board", "https://cdn.phenompeople.com/CareerConnectResources/pp/QUALUS/images/logo.png", "", "", "", false},
		{"phenom vendor site is not a board", "https://www.phenompeople.com/", "", "", "", false},
		{"phenom custom domain is not derivable", "https://careers.clearchoice.com/us/en/job/P-100864/Lead-Prosthodontist", "", "", "", false},

		// keyedsegment: Workstream's 8-hex employer id after /j/; an employer slug is not the board.
		{"workstream posting", "https://www.workstream.us/j/062be02e/baskin-robbins/los-angeles-21909/crew-member-2-eadf20a0?locale=en&referer_source=Applicant", "workstream", "062be02e", "https://www.workstream.us/j/062be02e", true},
		{"workstream employer root", "https://www.workstream.us/j/965a796b", "workstream", "965a796b", "https://www.workstream.us/j/965a796b", true},
		{"workstream employer slug is not the board", "https://www.workstream.us/j/finecasual", "", "", "", false},
		{"workstream marketing page has no board", "https://www.workstream.us/pricing", "", "", "", false},

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
		// jobscore: the company shortcode behind the platform's "careers" or "jobs" word.
		{"jobscore posting", "https://careers.jobscore.com/careers/ariasystems/jobs/implementation-consultant-a6n7LJEMraPQGI8xheCJ7l?ref=rss&sid=68", "jobscore", "ariasystems", "https://careers.jobscore.com/careers/ariasystems/jobs/implementation-consultant-a6n7LJEMraPQGI8xheCJ7l", true},
		{"jobscore feed", "https://careers.jobscore.com/jobs/ierustechnologiesinc/feed.json?sort=date", "jobscore", "ierustechnologiesinc", "https://careers.jobscore.com/jobs/ierustechnologiesinc/feed.json", true},
		{"jobscore bare careers has no board", "https://careers.jobscore.com/careers", "", "", "", false},
		// erecruiter: the cfg token, kept as spelled.
		{"erecruiter posting", "https://skk.erecruiter.pl/Offer.aspx?oid=4885221&cfg=d9f100d80dcc4d9895201c2993c46816&ejoId=526476&ejorId=216430&comId=20134027", "erecruiter", "d9f100d80dcc4d9895201c2993c46816", "https://skk.erecruiter.pl/GetHtml.ashx?cfg=d9f100d80dcc4d9895201c2993c46816&grid=rows&pn=1", true},
		{"erecruiter upper-case cfg kept as spelled", "https://skk.erecruiter.pl/Offer.aspx?oid=4978589&cfg=4D3DD27FCA3D40C79E01BEE9745902B7&ejoId=611624", "erecruiter", "4D3DD27FCA3D40C79E01BEE9745902B7", "https://skk.erecruiter.pl/GetHtml.ashx?cfg=4D3DD27FCA3D40C79E01BEE9745902B7&grid=rows&pn=1", true},
		{"erecruiter malformed cfg is not a board", "https://skk.erecruiter.pl/Offer.aspx?oid=1&cfg=acme", "", "", "", false},
		// eightfold: "<careers host>/<tenant domain>", the domain from the `domain` parameter only.
		{"eightfold career site", "https://eaton.eightfold.ai/careers?domain=eaton.com", "eightfold", "eaton.eightfold.ai/eaton.com", "https://eaton.eightfold.ai/careers?domain=eaton.com", true},
		{"eightfold posting", "https://infineon.eightfold.ai/careers/job/563087396?domain=infineon.com&hl=en", "eightfold", "infineon.eightfold.ai/infineon.com", "https://infineon.eightfold.ai/careers?domain=infineon.com", true},
		{"eightfold posting without domain is not derivable", "https://paypal.eightfold.ai/careers/job/274922819333", "", "", "", false},
		{"eightfold microsite is not the tenant domain", "https://albemarle.eightfold.ai/careers/job/1099553928573?microsite=albemarle.com", "", "", "", false},
		{"eightfold app host is not a board", "https://app.eightfold.ai/careers?domain=eaton.com", "", "", "", false},
		{"eightfold bare apex is not a board", "https://eightfold.ai/careers?domain=eaton.com", "", "", "", false},

		// pathpattern: the board at a fixed, shaped position behind the platform's own words.
		{"paycom portal posting", "https://www.paycomonline.net/v4/ats/web.php/portal/0c46774eb61f343c911be787715ae6f2/jobs/61570", "paycom", "0c46774eb61f343c911be787715ae6f2", "https://www.paycomonline.net/v4/ats/web.php/portal/0c46774eb61f343c911be787715ae6f2/career-page", true},
		{"paycom upper-case portal key kept as spelled", "https://www.paycomonline.net/v4/ats/web.php/portal/1A42347BA146BDD884AC51A0BDA04148/jobs/35720", "paycom", "1A42347BA146BDD884AC51A0BDA04148", "https://www.paycomonline.net/v4/ats/web.php/portal/1A42347BA146BDD884AC51A0BDA04148/career-page", true},
		{"paycom legacy clientkey link", "https://www.paycomonline.net/v4/ats/web.php/jobs/ViewJobDetails?job=2844&clientkey=0000007733F608D84DFE1BB6118E38D3", "paycom", "0000007733f608d84dfe1bb6118e38d3", "https://www.paycomonline.net/v4/ats/web.php/portal/0000007733f608d84dfe1bb6118e38d3/career-page", true},
		{"paycom truncated portal key is not a board", "https://www.paycomonline.net/v4/ats/web.php/portal/0c46774eb61f/jobs/1", "", "", "", false},
		{"paycom marketing page", "https://www.paycom.com/careers/", "", "", "", false},
		{"comeet board listing", "https://www.comeet.com/jobs/aitan/FA.006", "comeet", "aitan/FA.006", "https://www.comeet.com/jobs/aitan/FA.006", true},
		{"comeet posting", "https://www.comeet.com/jobs/Dario.Health/34.006/senior-backend-engineer/9B.F62", "comeet", "Dario.Health/34.006", "https://www.comeet.com/jobs/Dario.Health/34.006", true},
		{"comeet .co host", "https://www.comeet.co/jobs/trullion/07.000", "comeet", "trullion/07.000", "https://www.comeet.co/jobs/trullion/07.000", true},
		{"comeet slug without uid", "https://www.comeet.com/jobs/aitan", "", "", "", false},
		{"comeet vendor page", "https://www.comeet.com/pricing/plans/x", "", "", "", false},
		{"neogov agency posting", "https://www.governmentjobs.com/careers/ALBANYOR/jobs/5179173/on-call-transit-operator", "neogov", "governmentjobs.com/ALBANYOR", "https://www.governmentjobs.com/careers/ALBANYOR", true},
		{"neogov schooljobs posting", "https://www.schooljobs.com/careers/cochisecollege/jobs/5232702/pt-lifelong-learning-instructor", "neogov", "schooljobs.com/cochisecollege", "https://www.schooljobs.com/careers/cochisecollege", true},
		{"neogov agency home", "https://www.governmentjobs.com/careers/champlin", "neogov", "governmentjobs.com/champlin", "https://www.governmentjobs.com/careers/champlin", true},
		{"neogov search page is not an agency", "https://www.governmentjobs.com/careers/home/index?agency=champlin", "", "", "", false},
		{"neogov posting outside an agency", "https://www.governmentjobs.com/jobs/5179173-0/on-call-transit-operator", "", "", "", false},
		{"geekhunter posting", "https://www.geekhunter.com/pt/bigens/jobs/controller-financeiro-2", "geekhunter", "bigens", "https://www.geekhunter.com/pt/bigens/jobs", true},
		{"geekhunter board listing", "https://www.geekhunter.com/pt/ntt-data/jobs", "geekhunter", "ntt-data", "https://www.geekhunter.com/pt/ntt-data/jobs", true},
		{"geekhunter page without jobs is not a board", "https://www.geekhunter.com/pt/vagas", "", "", "", false},
		{"cleverstaff company page", "https://cleverstaff.net/i/devabit", "cleverstaff", "devabit", "https://cleverstaff.net/i/devabit", true},
		{"cleverstaff vacancies alias", "https://cleverstaff.net/i/master-of-code-global-vacancies", "cleverstaff", "master-of-code-global", "https://cleverstaff.net/i/master-of-code-global", true},
		{"cleverstaff posting names no alias", "https://cleverstaff.net/i/vacancy-VDpQ85", "", "", "", false},
		{"cleverstaff lower-case posting id names no alias", "https://cleverstaff.net/i/vacancy-qhbrpo", "", "", "", false},
		{"cleverstaff app route is not a tenant", "https://cleverstaff.net/i/signup", "", "", "", false},
		{"cleverstaff deep app route is not a tenant", "https://cleverstaff.net/i/pv/abc123", "", "", "", false},

		{"unknown host", "https://example.com/careers/1", "", "", "", false},
		{"not http", "ftp://acme.recruitee.com", "", "", "", false},
		{"garbage", "not a url", "", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src, board, canon, ok := Recognize(c.raw)
			if ok != c.wantOK {
				t.Fatalf("Recognize(%q) ok = %v, want %v", c.raw, ok, c.wantOK)
			}
			if !ok {
				return
			}
			if src != c.wantSource || board != c.wantBoard || canon != c.wantCanonical {
				t.Errorf("Recognize(%q) = (%q, %q, %q), want (%q, %q, %q)",
					c.raw, src, board, canon, c.wantSource, c.wantBoard, c.wantCanonical)
			}
		})
	}
}

// TestVacancyAndListingSameBoard proves a vacancy URL and a bare board URL for the same company
// collapse to one board (both modes), so the second (any vacancy on it) is a duplicate.
func TestVacancyAndListingSameBoard(t *testing.T) {
	pairs := [][2]string{
		// path
		{"https://jobs.ashbyhq.com/blitzy/a741b4e8", "https://jobs.ashbyhq.com/blitzy"},
		{"https://job-boards.greenhouse.io/acme/jobs/1?utm=x", "https://job-boards.greenhouse.io/acme"},
		// pathlocale (Rippling): a locale-prefixed vacancy and the bare listing collapse to one board
		{"https://ats.rippling.com/en-GB/satomic/jobs/48384892", "https://ats.rippling.com/satomic"},
		// subdomain
		{"https://acme.recruitee.com/o/senior-go", "https://acme.recruitee.com"},
		{"https://acme.bamboohr.com/careers/42/detail", "https://acme.bamboohr.com/careers/list"},
		// host+path (Workday): a vacancy and the site landing collapse to one board
		{"https://gm.wd5.myworkdayjobs.com/Careers_GM/job/x/Eng_JR-1", "https://gm.wd5.myworkdayjobs.com/Careers_GM"},
		// pathlocalepair (Dayforce): a culture-prefixed vacancy and the bare site landing
		{"https://jobs.dayforcehcm.com/en-US/dcrusa/join-us/jobs/12345", "https://jobs.dayforcehcm.com/dcrusa/join-us"},
		// query (Paycor): a posting and the board's career home, which differ in path AND query
		{"https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=4028f88b24c330a20124eb33b4ad1653&id=8a7883a6", "https://recruitingbypaycor.com/career/CareerHome.action?clientId=4028f88b24c330a20124eb33b4ad1653"},
		// hosttenant (UKG Ready): a vacancy and its board's career page
		{"https://secure.entertimeonline.com/ta/10284.careers?ShowJob=123456", "https://secure.entertimeonline.com/ta/10284.careers"},
		// host (HiringThing) and subdomain (HRM Direct): a posting and the board root
		{"https://4mindsai.hiringthing.com/job/12345/senior-engineer", "https://4mindsai.hiringthing.com/"},
		{"https://123loadboard.hrmdirect.com/employment/job-opening.php?req=3456&req_loc=1029", "https://123loadboard.hrmdirect.com/employment/job-openings.php?search=true"},
		{"https://senca.hireclick.com/jb/family-development-associate-nebraska-city/view/264587", "https://senca.hireclick.com/jobboard/jobboard.aspx"},
		// path (Gusto): a paged listing and the bare board
		{"https://jobs.gusto.com/boards/100-degrees-consulting-ec8f0e9c-d6b4-4a32-8544-9c213ca6bc90?page=3", "https://jobs.gusto.com/boards/100-degrees-consulting-ec8f0e9c-d6b4-4a32-8544-9c213ca6bc90"},
		// pathpattern (NEOGOV, Paycom): a posting and the board's own listing
		{"https://www.governmentjobs.com/careers/champlin/jobs/5179173/analyst", "https://www.governmentjobs.com/careers/champlin"},
		{"https://www.paycomonline.net/v4/ats/web.php/portal/0c46774eb61f343c911be787715ae6f2/jobs/61570", "https://www.paycomonline.net/v4/ats/web.php/portal/0c46774eb61f343c911be787715ae6f2/career-page"},
		// query with a tenant host (Eightfold): a posting and the career site
		{"https://eaton.eightfold.ai/careers/job/1?domain=eaton.com", "https://eaton.eightfold.ai/careers?domain=eaton.com"},
	}
	for _, p := range pairs {
		sa, ba, _, oka := Recognize(p[0])
		sb, bb, _, okb := Recognize(p[1])
		if !oka || !okb || sa != sb || ba != bb {
			t.Errorf("boards diverged: (%q,%q,%v) vs (%q,%q,%v)", sa, ba, oka, sb, bb, okb)
		}
	}
}

// TestRecognizeMapsHostsToTheIngestProviderName pins that a recognised host resolves to the
// source key the CATALOGUE uses, not to a name derived from the domain. Getting this wrong is
// silently expensive: the board-tracked check looks jobs up by (source, board), so a board we
// already crawl under one name looks brand new under another — it is recorded as a fresh
// contribution and paid for, and board coverage finds no ingest adapter to read it with.
//
// Factorial is the case in point: one adapter serves <tenant>.factorial.<tld> and
// <tenant>.factorialhr.<tld>, and it reports "factorial" for both.
func TestRecognizeMapsHostsToTheIngestProviderName(t *testing.T) {
	cases := []struct{ raw, wantSource string }{
		{"https://muffin.factorial.it/job/1", "factorial"},
		{"https://9net.factorialhr.com.br/job/2", "factorial"},
		{"https://4farma.factorialhr.pt/job/3", "factorial"},
		{"https://etrade.gr8people.com/jobs/4709", "gr8people"},
		{"https://batesville.workgr8.com/jobs/1146", "gr8people"},
	}
	for _, c := range cases {
		src, _, _, ok := Recognize(c.raw)
		if !ok || src != c.wantSource {
			t.Errorf("Recognize(%q) source = %q (ok %v), want %q", c.raw, src, ok, c.wantSource)
		}
	}
}

// TestQueryModeRowsAreConfigured guards the modes whose rows are not self-contained: a query
// entry needs a second row in queryBoards naming the parameter, a querypair entry a row in
// queryPairBoards naming both, a keyedsegment entry a row in keyedSegmentBoards naming the key
// and the board shape, and a pathpattern entry a row in pathPatternBoards whose pattern captures
// exactly one group. Without it the host matches and then declines every link on it —
// a platform that looks supported and recognises nothing. The rest of the table keeps the
// "one row plus one test case" promise on its own.
func TestQueryModeRowsAreConfigured(t *testing.T) {
	for _, a := range atsBoards {
		switch a.mode {
		case modeQuery:
			if q, ok := queryBoards[a.host]; !ok || q.param == "" || q.listingPath == "" || q.boardPattern == nil {
				t.Errorf("atsBoards entry %q (%s) is %s mode but queryBoards has no complete entry for it",
					a.host, a.source, modeQuery)
			}
		case modeQueryPair:
			q, ok := queryPairBoards[a.host]
			if !ok || q.first == "" || q.second == "" || q.listingPath == "" ||
				q.firstPattern == nil || q.secondPattern == nil || q.foldFirst == nil {
				t.Errorf("atsBoards entry %q (%s) is %s mode but queryPairBoards has no complete entry for it",
					a.host, a.source, modeQueryPair)
			}
		case modeKeyedSegment:
			if k, ok := keyedSegmentBoards[a.host]; !ok || k.key == "" || k.boardPattern == nil {
				t.Errorf("atsBoards entry %q (%s) is %s mode but keyedSegmentBoards has no complete entry for it",
					a.host, a.source, modeKeyedSegment)
			}
		case modePathPattern:
			p, ok := pathPatternBoards[a.host]
			if !ok || p.path == nil || p.path.NumSubexp() != 1 || p.listing == "" || (p.param != "") != (p.paramPattern != nil) {
				t.Errorf("atsBoards entry %q (%s) is %s mode but pathPatternBoards has no complete entry for it",
					a.host, a.source, modePathPattern)
			}
		}
	}
}

// A host whose mode this package has not been taught to read must be DECLINED, not read by
// whichever rule happens to sit last. The first-path-segment rule used to be that fallback,
// so a platform added with a new mode — one whose board is a query parameter, a host label,
// or anything else — would have had the vendor's own leading path segment taken as a tenant:
// a board that does not exist, which the contribution flow records and pays for.
func TestRecognizeDeclinesAModeItCannotRead(t *testing.T) {
	original := atsBoards
	t.Cleanup(func() { atsBoards = original })
	atsBoards = append(append([]struct{ host, source, mode string }(nil), original...),
		struct{ host, source, mode string }{"jobs.notyetsupported.test", "notyetsupported", "amodenobodywrote"})

	raw := "https://jobs.notyetsupported.test/careers/acme/job/1"
	if src, board, canonical, ok := Recognize(raw); ok {
		t.Errorf("Recognize(%q) = (%q, %q, %q, ok=true), want declined — no rule reads this mode",
			raw, src, board, canonical)
	}
}

// TestRecognizeReturnsTheBoardTheBoardFileStores pins the extraction of the six platforms added
// most recently against boards copied VERBATIM out of the shipped sources/<provider>.yml, one
// per board-id shape each file contains.
//
// A rule that disagrees with the board file is worse than no rule at all. The board string is
// what the contribution flow records and pays for, and what board coverage hands to the ingest
// adapter — so a near-miss files an employer under a board no crawl can resolve, while looking
// exactly like a successful recognition. The board files are the only authority on the shape: a
// provider's crawl is built around it.
func TestRecognizeReturnsTheBoardTheBoardFileStores(t *testing.T) {
	cases := []struct{ name, raw, source, board string }{
		// paycor: the 32-hex clientId, lower-case in the file (the platform serves either).
		{"paycor Day Kimball Health", "https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=4028f88b24c330a20124eb33b4ad1653&id=1", "paycor", "4028f88b24c330a20124eb33b4ad1653"},
		{"paycor Clark Hill", "https://recruitingbypaycor.com/career/CareerHome.action?clientId=8a29a018478a87e70147a7588a6343cf", "paycor", "8a29a018478a87e70147a7588a6343cf"},

		// dayforce: "<tenant>/<site>", the two segments after the culture.
		{"dayforce 4Refuel", "https://jobs.dayforcehcm.com/en-US/4refuel/candidateportal", "dayforce", "4refuel/candidateportal"},
		{"dayforce 99 Cents Only Stores", "https://jobs.dayforcehcm.com/en-US/99cents/candidateportal/jobs/1", "dayforce", "99cents/candidateportal"},
		{"dayforce DCR USA", "https://jobs.dayforcehcm.com/dcrusa/join-us", "dayforce", "dcrusa/join-us"},

		// gusto: the whole "<company-slug>-<company-uuid>" segment.
		{"gusto 100 Degrees Consulting", "https://jobs.gusto.com/boards/100-degrees-consulting-ec8f0e9c-d6b4-4a32-8544-9c213ca6bc90", "gusto", "100-degrees-consulting-ec8f0e9c-d6b4-4a32-8544-9c213ca6bc90"},
		{"gusto 121G", "https://jobs.gusto.com/boards/121g-consulting-llc-f41b3691-0c93-4406-acb7-6b28c2c69601?page=1", "gusto", "121g-consulting-llc-f41b3691-0c93-4406-acb7-6b28c2c69601"},

		// hiringthing: the full careers host, on the vendor's domain and on three resellers'.
		{"hiringthing 4Minds", "https://4mindsai.hiringthing.com/job/1", "hiringthing", "4mindsai.hiringthing.com"},
		{"hiringthing 10 Federal", "https://10federal-partners-inc.prismhr-hire.com/job/1", "hiringthing", "10federal-partners-inc.prismhr-hire.com"},
		{"hiringthing Integral Senior Living", "https://1440-by-the-bay.oasisrecruit.com/job/1", "hiringthing", "1440-by-the-bay.oasisrecruit.com"},
		{"hiringthing BASH", "https://1235-keystone-way-llc.gnahiring.com/job/1", "hiringthing", "1235-keystone-way-llc.gnahiring.com"},

		// ukgready: "<host>/<tenant>", one per host family in the file (including the bare apex
		// and the alphanumeric tenant ids, which are not all numeric).
		{"ukgready Rapid Robert's", "https://secure.entertimeonline.com/ta/10284.careers?ShowJob=1", "ukgready", "secure.entertimeonline.com/10284"},
		{"ukgready Phoenix Zoo", "https://secure7.saashr.com/ta/6010265.careers", "ukgready", "secure7.saashr.com/6010265"},
		{"ukgready TotalTec", "https://saashr.com/ta/6199997.careers", "ukgready", "saashr.com/6199997"},
		{"ukgready Copper State Bolt & Nut", "https://secure3.yourpayrollhr.com/ta/ihr0568.careers", "ukgready", "secure3.yourpayrollhr.com/ihr0568"},
		{"ukgready SPC Global", "https://secure.workforceready.com.au/ta/6162382.careers", "ukgready", "secure.workforceready.com.au/6162382"},
		{"ukgready C. Caswell Engineering", "https://secure.workforceready.eu/ta/6121944.careers", "ukgready", "secure.workforceready.eu/6121944"},
		{"ukgready General Health System", "https://prd01-hcm01.npr.mykronos.com/ta/6042637.careers", "ukgready", "prd01-hcm01.npr.mykronos.com/6042637"},

		// hrmdirect: the tenant subdomain alone.
		{"hrmdirect 123Loadboard", "https://123loadboard.hrmdirect.com/employment/job-opening.php?req=1&req_loc=2", "hrmdirect", "123loadboard"},
		{"hrmdirect 1A Auto", "https://1aautoinc.hrmdirect.com/employment/job-openings.php?search=true", "hrmdirect", "1aautoinc"},

		// hireclick: the tenant subdomain alone.
		{"hireclick Southwest Truck & RV", "https://southwesttruckandrv.hireclick.com/jb/professional-diesel-technician-phoenix-az/view/267596", "hireclick", "southwesttruckandrv"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src, board, _, ok := Recognize(c.raw)
			if !ok || src != c.source || board != c.board {
				t.Errorf("Recognize(%q) = (%q, %q, ok=%v), want (%q, %q) — the board sources/%s.yml stores",
					c.raw, src, board, ok, c.source, c.board, c.source)
			}
		})
	}
}
