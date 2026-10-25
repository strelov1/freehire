package atsdetect

import "testing"

func TestFromURL(t *testing.T) {
	cases := []struct {
		name     string
		url      string
		provider string
		board    string
		ok       bool
	}{
		// Workday: board is host + the career-site segment, with the optional
		// locale segment (en-us) dropped.
		{
			name:     "workday with locale",
			url:      "https://trumpf.wd3.myworkdayjobs.com/en-us/trumpf_students/job/apodaca-mexico/ar-student_r00040838",
			provider: "workday", board: "trumpf.wd3.myworkdayjobs.com/trumpf_students", ok: true,
		},
		{
			name:     "workday without locale",
			url:      "https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite/job/US/role_JR123",
			provider: "workday", board: "nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite", ok: true,
		},
		{
			name:     "workday non-english locale",
			url:      "https://wmg.wd1.myworkdayjobs.com/de-DE/wmgglobal/job/berlin/artist_jr1",
			provider: "workday", board: "wmg.wd1.myworkdayjobs.com/wmgglobal", ok: true,
		},
		// SmartRecruiters: board is the company slug (may contain hyphens).
		{
			name:     "smartrecruiters",
			url:      "https://jobs.smartrecruiters.com/boschgroup/744000112541067-sr-staff-electrical",
			provider: "smartrecruiters", board: "boschgroup", ok: true,
		},
		{
			name:     "smartrecruiters hyphenated slug",
			url:      "https://jobs.smartrecruiters.com/atuauto-teile-unger/744000129168469-verkaufer",
			provider: "smartrecruiters", board: "atuauto-teile-unger", ok: true,
		},
		// Greenhouse: direct job-boards host plus the EU host.
		{
			name:     "greenhouse job-boards",
			url:      "https://job-boards.greenhouse.io/avepoint/jobs/6899160",
			provider: "greenhouse", board: "avepoint", ok: true,
		},
		{
			name:     "greenhouse eu host",
			url:      "https://job-boards.eu.greenhouse.io/amoriabond/jobs/4878751101",
			provider: "greenhouse", board: "amoriabond", ok: true,
		},
		// Lever / Ashby: board is the org slug.
		{
			name:     "lever",
			url:      "https://jobs.lever.co/findhelp/abc-123",
			provider: "lever", board: "findhelp", ok: true,
		},
		{
			name:     "ashby",
			url:      "https://jobs.ashbyhq.com/llamaindex/156d7573-82dd-4973-87ab-7d5a056650d5",
			provider: "ashby", board: "llamaindex", ok: true,
		},
		// Subdomain-keyed boards.
		{
			name:     "jazzhr",
			url:      "https://prismbiotech.applytojob.com/apply/7ohqbqa6mh/territory-rep",
			provider: "jazzhr", board: "prismbiotech", ok: true,
		},
		{
			name:     "recruitee",
			url:      "https://diegrenze.recruitee.com/o/hulpkracht-haaksbergen-1",
			provider: "recruitee", board: "diegrenze", ok: true,
		},
		{
			name:     "pinpoint",
			url:      "https://therapymgmt.pinpointhq.com/en/postings/88dfb239-bb43",
			provider: "pinpoint", board: "therapymgmt", ok: true,
		},
		{
			name:     "careerplug",
			url:      "https://golden-corral-careers.careerplug.com/jobs/1334156?utm_source=Role",
			provider: "careerplug", board: "golden-corral-careers", ok: true,
		},
		// ApplicantStack: board is the tenant subdomain, lower-cased.
		{
			name:     "applicantstack",
			url:      "https://mainenilesassociationofspecialrecreation.applicantstack.com/x/detail/a2pwbm9kwu0s",
			provider: "applicantstack", board: "mainenilesassociationofspecialrecreation", ok: true,
		},
		{
			name:     "applicantstack hyphenated tenant",
			url:      "https://ama-consult.applicantstack.com/x/detail/a261jevmq30q",
			provider: "applicantstack", board: "ama-consult", ok: true,
		},
		{
			name:     "applicantstack mixed-case tenant",
			url:      "https://SmartAutoCare.applicantstack.com/x/detail/a2g5b5u0kq8i",
			provider: "applicantstack", board: "smartautocare", ok: true,
		},
		{
			// A tenant on its own domain names no board the URL can reveal.
			name: "applicantstack on a custom domain",
			url:  "https://careers.millerfabricationsolutions.com/x/detail/plant-manager-pine-creek",
			ok:   false,
		},
		// HireClick: the board is the tenant subdomain; the slug before /view/<id> is decorative.
		{
			name:     "hireclick",
			url:      "https://blacktiecomponents.hireclick.com/jb/logistics-specialist-driver/view/268119",
			provider: "hireclick", board: "blacktiecomponents", ok: true,
		},
		{
			name:     "hireclick long slug",
			url:      "https://peninsulatotalcarcare.hireclick.com/jb/skilled-automotive-technician-b-level-production-technician-80-000-110-000-2k-sign-on-cash-5-yr-exp-req/view/263568",
			provider: "hireclick", board: "peninsulatotalcarcare", ok: true,
		},
		{
			name:     "hireclick vendor marketing host",
			url:      "https://employers.hireclick.com/",
			provider: "", board: "", ok: false,
		},
		{
			name:     "paycom",
			url:      "https://www.paycomonline.net/v4/ats/web.php/portal/3b4555a93baac45919556b5f901f7b83/jobs/383852?utm_source=Role",
			provider: "paycom", board: "3b4555a93baac45919556b5f901f7b83", ok: true,
		},
		{
			// A posting link names the client key as a query parameter instead of a path segment,
			// often upper-case and on a regional host; the catalogue holds the key lower-case.
			name:     "paycom clientkey query",
			url:      "https://phx.us-west.paycomonline.net/v4/ats/web.php/jobs/ViewJobDetails?job=2844&clientkey=0000007733F608D84DFE1BB6118E38D3",
			provider: "paycom", board: "0000007733f608d84dfe1bb6118e38d3", ok: true,
		},
		{
			name: "paycom malformed clientkey",
			url:  "https://www.paycomonline.net/v4/ats/web.php/jobs/ViewJobDetails?job=2844&clientkey=acme",
		},
		// iCIMS moved into the shared atsboard table (the reward it used to guard against no
		// longer exists), so both shapes reach FromURL through the delegation at its top —
		// see TestRecognize in atsboard for the full coverage, this is just a smoke test that
		// the delegation still wires through.
		{
			name:     "icims careers prefix",
			url:      "https://careers-hcsgcorp.icims.com/jobs/704470/dietary-aide/job",
			provider: "icims", board: "hcsgcorp", ok: true,
		},
		{
			name:     "icims vanity host without careers prefix",
			url:      "https://uk-external-novelis.icims.com/jobs/49037/mechanical-day-technician",
			provider: "icims", board: "uk-external-novelis.icims.com", ok: true,
		},
		// Oracle: board is host + the candidate-experience site name.
		{
			name:     "oracle",
			url:      "https://eiby.fa.em2.oraclecloud.com/hcmui/candidateexperience/en/sites/jobsearch/job/11175",
			provider: "oracle", board: "eiby.fa.em2.oraclecloud.com/jobsearch", ok: true,
		},
		// Taleo: board is the tenant host + the careersection number.
		{
			name:     "taleo careersection",
			url:      "https://valero.taleo.net/careersection/2/jobsearch.ftl?lang=en&portal=101",
			provider: "taleo", board: "valero.taleo.net/2", ok: true,
		},
		{
			name: "taleo without careersection segment is not harvestable",
			url:  "https://valero.taleo.net/",
			ok:   false,
		},
		// Cornerstone (CSOD): board is the tenant subdomain of *.csod.com.
		{
			name:     "cornerstone csod",
			url:      "https://nintendoeurope.csod.com/ux/ats/careersite/1/home?c=nintendoeurope",
			provider: "cornerstone", board: "nintendoeurope", ok: true,
		},
		{
			name: "cornerstone multi-label subdomain is not harvestable",
			url:  "https://uk-ext.eu.csod.com/ux/ats/careersite/1/home",
			ok:   false,
		},
		// PageUp: board is the numeric institution id on the canonical host.
		{
			name:     "pageup canonical host",
			url:      "https://careers.pageuppeople.com/513/cw/en/job/694504/senior-audiovisual-design-engineer",
			provider: "pageup", board: "513", ok: true,
		},
		{
			name: "pageup non-numeric first segment is not a board",
			url:  "https://careers.pageuppeople.com/cw/en/search",
			ok:   false,
		},
		// NEOGOV: board is "<domain>/<agency>"; domain distinguishes the two tenant spaces.
		{
			name:     "neogov schooljobs",
			url:      "https://www.schooljobs.com/careers/cochisecollege/jobs/5371857/ft-academic-career-advisor-svc",
			provider: "neogov", board: "schooljobs.com/cochisecollege", ok: true,
		},
		{
			name:     "neogov governmentjobs",
			url:      "https://www.governmentjobs.com/careers/longbeach/jobs/4812345/analyst",
			provider: "neogov", board: "governmentjobs.com/longbeach", ok: true,
		},
		{
			name: "neogov without careers agency segment is not harvestable",
			url:  "https://www.governmentjobs.com/careers",
			ok:   false,
		},
		// ADP MyJobs is its own product with its own adapter: the slug is an adpmyjobs board,
		// never an adp one (whose board is the cid:ccId pair).
		{
			name:     "adp myjobs slug is adpmyjobs, not adp",
			url:      "https://myjobs.adp.com/afcindustries/cx/job-details?reqid=5001150982906",
			provider: "adpmyjobs", board: "afcindustries", ok: true,
		},
		// Shapes whose URL does not yield the board id our adapter expects.
		{
			name: "join slug not numeric company id",
			url:  "https://join.com/companies/hr-groep/16130508-metselaar",
			ok:   false,
		},
		{
			name: "workable job shortlink has no company slug",
			url:  "https://apply.workable.com/j/EF5014296F/apply",
			ok:   false,
		},
		{
			name:     "comeet vacancy keeps slug and company uid",
			url:      "https://www.comeet.com/jobs/capslock/59.001/senior-marketing-strategist/9B.F62-B8.406",
			provider: "comeet",
			board:    "capslock/59.001",
			ok:       true,
		},
		{
			name:     "comeet board listing",
			url:      "https://www.comeet.com/jobs/overwolf/B1.001",
			provider: "comeet",
			board:    "overwolf/B1.001",
			ok:       true,
		},
		{
			name: "comeet slug without a company uid is not a board",
			url:  "https://www.comeet.com/jobs/overwolf",
			ok:   false,
		},
		{
			name: "comeet vendor path is not a board",
			url:  "https://www.comeet.com/pricing/plans/x",
			ok:   false,
		},
		// Apply links from an external harvest, through the shared table: each board below was
		// crawled live through its adapter and yielded postings.
		{
			name:     "workstream employer id",
			url:      "https://www.workstream.us/j/3a856bab/culvers/eureka-77683/porter-b645d3a7?locale=en&referer_source=Applicant",
			provider: "workstream", board: "3a856bab", ok: true,
		},
		{
			name:     "feishu tenant host",
			url:      "https://bcn95a8dacj3.jobs.feishu.cn/campus/position/7695239842557020425/detail",
			provider: "feishujobs", board: "bcn95a8dacj3.jobs.feishu.cn", ok: true,
		},
		{
			name:     "successfactors sap-hosted site",
			url:      "https://mhh.jobs.hr.cloud.sap/job/Hannover-Duales-Studium-Business-Economics-%28wdm%29-NI-30625/1373094055/",
			provider: "successfactors", board: "mhh.jobs.hr.cloud.sap", ok: true,
		},
		{
			name:     "phenom vendor-hosted site",
			url:      "https://siemens.phenompro.com/global/en/job/R-31293/Nuclear-Pharmacist-training-provided-Birmingham-AL",
			provider: "phenom", board: "siemens.phenompro.com", ok: true,
		},
		{
			name:     "eploy vendor-hosted site",
			url:      "https://hadfabltdweb.eploy.net/vacancies/2/project-manager.html",
			provider: "eploy", board: "hadfabltdweb.eploy.net", ok: true,
		},
		// Harvested shapes that do not carry the board the adapter crawls: a posting id only
		// (gusto, schoolspring, edjoin), a product the adapter does not read (the legacy
		// SuccessFactors career section), or a custom domain with no signature that is the
		// platform's alone (eploy, phenom).
		{name: "gusto posting", url: "https://jobs.gusto.com/postings/wayback-burgers-back-and-front-of-house-team-memeber-52646664-c1a9-4098-9f14-d5119eaf9ff3"},
		{name: "schoolspring posting", url: "https://www.schoolspring.com/jobdetail?jobId=5962033"},
		{name: "edjoin posting", url: "https://www.edjoin.org/Home/JobPosting/2284252"},
		{name: "successfactors legacy career section", url: "https://career10.successfactors.com/career?career_ns=job_listing&company=ptmetrodat&career_job_req_id=39731"},
		{name: "eploy custom domain", url: "https://careers.simplyhealth.co.uk/vacancies/3094/financial-planning--analysis-partner.html"},
		{name: "phenom custom domain", url: "https://jobs.kwiktrip.com/us/en/job/231967/Customer-Food-Service-Overnight"},
		// GetHired: the board is the tenant subdomain of easyapply.co. The platform's apply
		// links sit on the bare apex (/a/<uuid>, /job/<slug>) and name no tenant — the tenant
		// is only reachable by fetching the page — so they are declined, not guessed.
		{
			name:     "gethired career site",
			url:      "https://freightbreakersllc.easyapply.co/",
			provider: "gethired", board: "freightbreakersllc", ok: true,
		},
		{
			name:     "gethired career site share link",
			url:      "https://whiteoakpostacutecare7568.easyapply.co",
			provider: "gethired", board: "whiteoakpostacutecare7568", ok: true,
		},
		{
			name: "gethired apply link names no tenant",
			url:  "https://easyapply.co/a/a0a12168-017b-4c25-837c-519d9103c453",
			ok:   false,
		},
		{
			name: "gethired posting slug names no tenant",
			url:  "https://easyapply.co/job/produce-clerk-120",
			ok:   false,
		},
		// Tenstreet: real apply links, the board is the carrier's numeric company_id. The
		// carriers' other apply host, intelliapp.driverapponline.com/c/<slug>, names no
		// company id and is declined.
		{
			name:     "tenstreet posting",
			url:      "https://pulse.tenstreet.com/share_app_show_active_jobs.php?active_job_id=123622&company_id=774&portal_user_id=1",
			provider: "tenstreet", board: "774", ok: true,
		},
		{
			name:     "tenstreet posting, seven-digit company",
			url:      "https://pulse.tenstreet.com/share_app_show_active_jobs.php?active_job_id=53720&company_id=2337095&portal_user_id=1",
			provider: "tenstreet", board: "2337095", ok: true,
		},
		{
			name:     "tenstreet posting, four-digit company",
			url:      "https://pulse.tenstreet.com/share_app_show_active_jobs.php?active_job_id=116117&company_id=2272&portal_user_id=1",
			provider: "tenstreet", board: "2272", ok: true,
		},
		{
			name:     "tenstreet posting, 3665093",
			url:      "https://pulse.tenstreet.com/share_app_show_active_jobs.php?active_job_id=130245&company_id=3665093&portal_user_id=1",
			provider: "tenstreet", board: "3665093", ok: true,
		},
		{
			name:     "tenstreet posting, 3041402",
			url:      "https://pulse.tenstreet.com/share_app_show_active_jobs.php?active_job_id=126863&company_id=3041402&portal_user_id=1",
			provider: "tenstreet", board: "3041402", ok: true,
		},
		{
			name:     "tenstreet carrier profile",
			url:      "https://pulse.tenstreet.com/share_app_index.php?company_id=140868",
			provider: "tenstreet", board: "140868", ok: true,
		},
		{
			name: "tenstreet intelliapp link names no company",
			url:  "https://intelliapp.driverapponline.com/c/hawaiitrans",
			ok:   false,
		},
		{
			name: "tenstreet posting without a company id",
			url:  "https://pulse.tenstreet.com/share_app_show_active_jobs.php?active_job_id=123622&portal_user_id=1",
			ok:   false,
		},
		// IdealTraits: the career-site link carries the board (the business id, base64-encoded);
		// an apply link names only the employer's slug and a posting id, so it carries none —
		// boardresolve reaches the board by fetching the posting page, which links the career
		// site. Both URLs are real.
		{
			name:     "idealtraits career site",
			url:      "https://app.idealtraits.com/Al-Bourdeau-Insurance-Agency/MTA3NjY=/careers",
			provider: "idealtraits", board: "10766", ok: true,
		},
		{
			name: "idealtraits apply link carries no board",
			url:  "https://app.idealtraits.com/career/Al-Bourdeau-Insurance-Agency/370429CPG",
			ok:   false,
		},
		// HigherMe: the board is the brand id on its career page. An apply link names only a
		// posting id (these three are real apply links); the brand behind it lives in the
		// posting's API record, so the URL alone has no board to give.
		{
			name:     "higherme brand career page",
			url:      "https://app.higherme.com/careers/6078977d8dfec",
			provider: "higherme", board: "6078977d8dfec", ok: true,
		},
		{
			name: "higherme apply link names a posting, not a brand",
			url:  "https://app.higherme.com/jobs/607aff1be9251",
			ok:   false,
		},
		{
			name: "higherme apply link 2",
			url:  "https://app.higherme.com/jobs/691b96a8e5001",
			ok:   false,
		},
		{
			name: "higherme apply link 3",
			url:  "https://app.higherme.com/jobs/6acfccec3d0b8",
			ok:   false,
		},
		// Paradox: board is the instance host + the career site, lower-cased.
		{
			name:     "paradox posting",
			url:      "https://olivia.paradox.ai/co/Wingstop161/Job?job_id=P1-2507154-1",
			provider: "paradox", board: "olivia.paradox.ai/wingstop161", ok: true,
		},
		{
			name:     "paradox eu instance",
			url:      "https://olivia.eu1.paradox.ai/co/Pandora/Job?job_id=PDX_PA_A22F58C3-7DC3-4E13-90A2-1F54B2381FDE_22070543",
			provider: "paradox", board: "olivia.eu1.paradox.ai/pandora", ok: true,
		},
		{
			// McDonald's white-label of the platform is a different host family; not a
			// paradox.ai instance, so it is not read as one.
			name: "mchire is not a paradox instance",
			url:  "https://www.mchire.com/co/McDonalds2523/Job?job_id=PDX_MC_C8C45FE7-AF20-4638-AC29-65BF8F219B8E_76278",
			ok:   false,
		},
		// Taleo Business Edition: <pod>.tbe.taleo.net, board = "<ORG>:<cws>" from the query, never
		// the pod. Taleo Enterprise (<tenant>.taleo.net/careersection/<n>) is a different product
		// and keeps resolving to the "taleo" adapter.
		{
			name:     "taleobe legacy posting",
			url:      "https://tre.tbe.taleo.net/tre01/ats/careers/requisition.jsp?org=SAINTELIZABETH&cws=50&rid=44356",
			provider: "taleobe", board: "SAINTELIZABETH:50", ok: true,
		},
		{
			name:     "taleobe v2 apply link",
			url:      "https://phh.tbe.taleo.net/phh03/ats/careers/v2/applyRequisition?org=MILCORP&cws=38&rid=16348",
			provider: "taleobe", board: "MILCORP:38", ok: true,
		},
		{
			name:     "taleobe requisition on another pod",
			url:      "https://phg.tbe.taleo.net/phg04/ats/careers/requisition.jsp?org=FDI&cws=41&rid=613",
			provider: "taleobe", board: "FDI:41", ok: true,
		},
		{
			name:     "taleo enterprise careersection stays taleo",
			url:      "https://valero.taleo.net/careersection/2/jobdetail.ftl?job=123",
			provider: "taleo", board: "valero.taleo.net/2", ok: true,
		},
		{
			name: "unknown host",
			url:  "https://www.caci.com/careers/123",
			ok:   false,
		},
		{
			name: "garbage url",
			url:  "not a url",
			ok:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, b, ok := FromURL(tc.url)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v (got provider=%q board=%q)", ok, tc.ok, p, b)
			}
			if ok && (p != tc.provider || b != tc.board) {
				t.Errorf("got (%q, %q), want (%q, %q)", p, b, tc.provider, tc.board)
			}
		})
	}
}

// TestFromURLHarriApplyLinks feeds real Harri apply links (harri.com/<brand slug>/job/<id>-,
// collected 2026-10-20) through FromURL. Every slug here resolved to a live brand through the
// platform's slug lookup, so each is the board the adapter crawls.
func TestFromURLHarriApplyLinks(t *testing.T) {
	cases := []struct{ url, board string }{
		{"https://harri.com/gk-beo1m4ztp4bs/job/2839063-", "gk-beo1m4ztp4bs"},
		{"https://harri.com/bicos/job/2793867-", "bicos"},
		{"https://harri.com/jollibeegroupna/job/2842103-", "jollibeegroupna"},
		{"https://harri.com/brindisakitchenscareers/job/2836139-", "brindisakitchenscareers"},
		{"https://harri.com/sa-asian-concepts-llc-dba-jollibee/job/2829329-", "sa-asian-concepts-llc-dba-jollibee"},
		{"https://harri.com/ghaijobs/job/2843117-", "ghaijobs"},
		{"https://harri.com/5star/job/2841805-", "5star"},
		{"https://harri.com/the-arts-club-38/job/2840208-", "the-arts-club-38"},
		{"https://harri.com/wendysukcareers/job/2835499-", "wendysukcareers"},
		{"https://harri.com/kgk-enterprises-dba-papa-john-s-us/job/2838358-", "kgk-enterprises-dba-papa-john-s-us"},
		{"https://harri.com/kjsdhflkcxnckjxhfnkjdhxfncdkgvjnrhcfjxhmkhjsdfc/job/2842490-", "kjsdhflkcxnckjxhfnkjdhxfncdkgvjnrhcfjxhmkhjsdfc"},
	}
	for _, c := range cases {
		provider, board, ok := FromURL(c.url)
		if !ok || provider != "harri" || board != c.board {
			t.Errorf("FromURL(%q) = (%q, %q, %v), want (harri, %q, true)", c.url, provider, board, ok, c.board)
		}
	}
}

// TestFromURLHCSHiring pins real apply links to symplr career centers (sampled 2026-10-20):
// every <tenant>.hcshiring.com/jobs/<id> link names its tenant as the hcshiring board, and the
// vendor's other products, which share no API with it, stay unrecognised.
func TestFromURLHCSHiring(t *testing.T) {
	boards := []struct{ url, board string }{
		{"https://capricommunities.hcshiring.com/jobs/TugN1ntScEij8OCB6I_Ezw", "capricommunities"},
		{"https://midwesthealth.hcshiring.com/jobs/7qxWmZzGVkKV65Juq1AJmg", "midwesthealth"},
		{"https://homewood.hcshiring.com/jobs/QipljehazEeuiHgKt-hX6Q", "homewood"},
		{"https://hawkeyecare.hcshiring.com/jobs/uOXXNZ8LBUWtzWh1NK3MrA", "hawkeyecare"},
		{"https://adfinitashealth.hcshiring.com/jobs/b8bT4Kwb0kGilISu7frcbA", "adfinitashealth"},
		{"https://sandyridgerehab.hcshiring.com/jobs/S8dfJO8TA0W7C7Gu2H5UlA", "sandyridgerehab"},
		{"https://riverviewlodgeal.hcshiring.com/jobs/_YMF6czAe0yKvrkSlh0NqA", "riverviewlodgeal"},
		{"https://orchardhealthandrehabilitation.hcshiring.com/jobs/PC6BEtLDg0GzxcouNRN0VA", "orchardhealthandrehabilitation"},
		{"https://paddingtonplacecottages.hcshiring.com/jobs/E4r5Pd4AFEarHTai6-l77Q", "paddingtonplacecottages"},
		{"https://birchhillhs.hcshiring.com/jobs/KLN--9GXqEW8LNqAiWqp2A", "birchhillhs"},
		{"https://regencymorristown.hcshiring.com/jobs/x3-ViUISqkimatByP1GJKA", "regencymorristown"},
		{"https://well5as.hcshiring.com/jobs/nZPRrHO_1UaArr4-Il5CzA", "well5as"},
		{"https://myrtlepoint.hcshiring.com/jobs/hWdmukUkFUCp5gEI6RUP_Q", "myrtlepoint"},
		{"https://chestnut.hcshiring.com/jobs/lT4J6VKsb02SfTYgkmkhQA", "chestnut"},
		{"https://howardvillage.hcshiring.com/jobs/ANwpKMNmR0S4WjoeJsgt-g", "howardvillage"},
		{"https://wisconsinrapidshs.hcshiring.com/jobs/hO31qdIjNESM2pYG6haCDA", "wisconsinrapidshs"},
		{"https://goldensands.hcshiring.com/jobs/6t_OchT_sE-vdMQKYfJUqQ", "goldensands"},
		{"https://traditionsofhershey.hcshiring.com/jobs/YFoBcXpOeE23j39SH3Wq7Q", "traditionsofhershey"},
		{"https://calveriahartville.hcshiring.com/jobs/c8E9SO2-EEGEOzuxQhhDVA", "calveriahartville"},
		{"https://regencypark.hcshiring.com/jobs/h0Qlrs3slkePTXuxRW1-XQ", "regencypark"},
		{"https://countrylaneicf.hcshiring.com/jobs/pFV0DM_ONUiRDqDQYirYGw", "countrylaneicf"},
		{"https://soldiersgrovehs.hcshiring.com/jobs/uQKX74IrGE2YwAMTNLpqVQ", "soldiersgrovehs"},
		{"https://alamar.hcshiring.com/jobs/CB45JUXffkWVPGBlBT5cNQ", "alamar"},
		{"https://harringtonparkhealthandrehab.hcshiring.com/jobs/V8XzEs2tdkOW5FDwCovQow", "harringtonparkhealthandrehab"},
		{"https://bolingreenhealthandrehabilitation.hcshiring.com/jobs/wKyNItlptkKsQw6SdZH6bg", "bolingreenhealthandrehabilitation"},
		{"https://solarishealthcarelakezephyr.hcshiring.com/jobs/k8vUK4WLd0Gsfm3R1duH3Q", "solarishealthcarelakezephyr"},
		{"https://kensington.hcshiring.com/jobs/5R4PGRmW5kehEodflpoljA", "kensington"},
		{"https://agewellsolvere.hcshiring.com/jobs/eBs-usikvE-Ojl8-ton2kQ", "agewellsolvere"},
		{"https://capricommunitiesaccount.hcshiring.com/jobs/I0JmSDI2LkOLb5Vjj1d8kg", "capricommunitiesaccount"},
		{"https://tampalakes.hcshiring.com/jobs/kBdbVUxqo0O79kJ9xm7Lxw", "tampalakes"},
	}
	for _, c := range boards {
		provider, board, ok := FromURL(c.url)
		if !ok || provider != "hcshiring" || board != c.board {
			t.Errorf("FromURL(%q) = (%q, %q, %v), want (hcshiring, %q, true)", c.url, provider, board, ok, c.board)
		}
	}

	for _, u := range []string{
		"https://valleywise.hctsportals.com/jobs/2237498",
		"https://jobs-pediatricassociates.hctsportals.com/jobs/2224904",
		"https://pm.healthcaresource.com/CS/rrh/#/job/2474",
		"https://pm.healthcaresource.com/CS/exeterhospital/#/job/13541",
	} {
		if provider, board, ok := FromURL(u); ok {
			t.Errorf("FromURL(%q) = (%q, %q), want unrecognised: another symplr product", u, provider, board)
		}
	}
}
