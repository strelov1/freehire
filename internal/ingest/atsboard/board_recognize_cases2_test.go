package atsboard

// recognizeCases2 is part 2 of 7 of the TestRecognize table, in order.
var recognizeCases2 = []recognizeCase{
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
	{"taleo enterprise is a different product, read as taleo", "https://valero.taleo.net/careersection/2/jobdetail.ftl?job=123&org=X&cws=1", "taleo", "valero.taleo.net/2", "https://valero.taleo.net/careersection/2/jobsearch.ftl", true},
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
	{"trinethire posting", "https://app.trinethire.com/companies/16150-automox-inc/jobs/26497-senior-software-engineer-backend?source=LinkedIn", "trinethire", "16150-automox-inc", "https://app.trinethire.com/companies/16150-automox-inc/jobs", true},
	{"trinethire segment without a company id is not a board", "https://app.trinethire.com/companies/automox/jobs", "", "", "", false},
	{"trinethire product page is not a board", "https://app.trinethire.com/privacy", "", "", "", false},
	{"gohire posting", "https://jobs.gohire.io/dexerto-de5jlhjo/marketing-manager-304991/", "gohire", "dexerto-de5jlhjo", "https://jobs.gohire.io/dexerto-de5jlhjo/", true},
	{"gohire retired numeric form is not a board", "https://jobs.gohire.io/dexerto-19265/", "", "", "", false},
	{"gohire static asset is not a board", "https://jobs.gohire.io/assets/js/boardScript.js", "", "", "", false},
	{"manatal tenant subdomain listing", "https://24-mag.careers-page.com/", "manatal", "24-mag", "https://24-mag.careers-page.com", true},
	{"manatal tenant subdomain posting is not a board called job", "https://24-mag.careers-page.com/job/5W939XW3", "manatal", "24-mag", "https://24-mag.careers-page.com", true},
	{"hireology careers host", "https://creativesolutionsinhealthcare.hireology.careers/#Jobs", "hireology", "creativesolutionsinhealthcare", "https://creativesolutionsinhealthcare.hireology.careers", true},
	{"jibe on the vendor domain", "https://cipd.jibeapply.com/jobs", "jibe", "cipd.jibeapply.com", "https://cipd.jibeapply.com", true},
	{"taleo careersection", "https://aa020.taleo.net/careersection/cuhkmc/jobsearch.ftl?lang=en", "taleo", "aa020.taleo.net/cuhkmc", "https://aa020.taleo.net/careersection/cuhkmc/jobsearch.ftl", true},
	{"taleo posting", "https://etisalatmisr.taleo.net/careersection/ex/jobdetail.ftl?job=12345", "taleo", "etisalatmisr.taleo.net/ex", "https://etisalatmisr.taleo.net/careersection/ex/jobsearch.ftl", true},
	{"taleo section with dots and dashes", "https://nexcom.taleo.net/careersection/.nexcom_ext_prof_cs/jobsearch.ftl", "taleo", "nexcom.taleo.net/.nexcom_ext_prof_cs", "https://nexcom.taleo.net/careersection/.nexcom_ext_prof_cs/jobsearch.ftl", true},
	{"taleo rest tree is not a section", "https://acme.taleo.net/careersection/rest/jobboard/searchjobs", "", "", "", false},
	{"oracle candidate experience site", "https://epwk.fa.us2.oraclecloud.com/hcmUI/CandidateExperience/en/sites/CX/jobs", "oracle", "epwk.fa.us2.oraclecloud.com/CX", "https://epwk.fa.us2.oraclecloud.com/hcmUI/CandidateExperience/en/sites/CX", true},
	{"oracle posting", "https://ekcm.fa.us6.oraclecloud.com/hcmUI/CandidateExperience/en-GB/sites/CX_1001/job/42", "oracle", "ekcm.fa.us6.oraclecloud.com/CX_1001", "https://ekcm.fa.us6.oraclecloud.com/hcmUI/CandidateExperience/en/sites/CX_1001", true},
	{"oracle host outside candidate experience", "https://epwk.fa.us2.oraclecloud.com/fscmUI/faces/FuseWelcome", "", "", "", false},
	{"loxo shared host board", "https://app.loxo.co/AKRE-Associates", "loxo", "app.loxo.co/AKRE-Associates", "https://app.loxo.co/AKRE-Associates", true},
	{"loxo agency host board", "https://fitnext.app.loxo.co/fitnext", "loxo", "fitnext.app.loxo.co/fitnext", "https://fitnext.app.loxo.co/fitnext", true},
	{"loxo posting names no agency", "https://app.loxo.co/job/MjgwMDctem15YTg1MTJ0eWQwbjhyNQ==", "", "", "", false},
	{"loxo app route is not an agency", "https://app.loxo.co/login", "", "", "", false},
	{"recruiterflow board", "https://recruiterflow.com/sagepaths/jobs", "recruiterflow", "sagepaths", "https://recruiterflow.com/sagepaths/jobs", true},
	{"recruiterflow posting", "https://recruiterflow.com/sagepaths/jobs/1234", "recruiterflow", "sagepaths", "https://recruiterflow.com/sagepaths/jobs", true},
	{"recruiterflow vendor page", "https://recruiterflow.com/pricing", "", "", "", false},
	{"paylocity bare listing path is not a board", "https://recruiting.paylocity.com/Recruiting/Jobs/All", "", "", "", false},
}
