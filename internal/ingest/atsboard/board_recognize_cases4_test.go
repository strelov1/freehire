package atsboard

// recognizeCases4 is part 4 of 7 of the TestRecognize table, in order.
var recognizeCases4 = []recognizeCase{
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
	// Since 2026-10-21 ClearCompany serves every HRM Direct tenant: <board>.hrmdirect.com
	// redirects to apply.clearcompany.com/<board>/redirect, which lands on the career page
	// apply.clearcompany.com/<board>, and a posting is that page with ?jobId=<guid>. The slug
	// is the board the catalogue already holds, lower-case; the platform answers any case.
	{"clearcompany career page", "https://apply.clearcompany.com/sms", "hrmdirect", "sms", "https://apply.clearcompany.com/sms", true},
	{"clearcompany posting", "https://apply.clearcompany.com/bkbehavior?jobId=3522713d-0ff6-5e08-72c3-617411eaccb5", "hrmdirect", "bkbehavior", "https://apply.clearcompany.com/bkbehavior", true},
	{"clearcompany redirect landing", "https://apply.clearcompany.com/123loadboard/redirect?req=3456", "hrmdirect", "123loadboard", "https://apply.clearcompany.com/123loadboard", true},
	{"clearcompany upper-case slug folds", "https://apply.clearcompany.com/SMS/?noheader=False", "hrmdirect", "sms", "https://apply.clearcompany.com/sms", true},
	{"clearcompany bare host no tenant", "https://apply.clearcompany.com/", "", "", "", false},
	{"clearcompany deeper path is not a career page", "https://apply.clearcompany.com/sms/jobs", "", "", "", false},
	{"clearcompany careers path is not a board", "https://apply.clearcompany.com/Careers", "", "", "", false},
	{"clearcompany marketing site is not a board", "https://www.clearcompany.com/applicant-tracking", "", "", "", false},
	{"clearcompany api host is not a board", "https://careers-api.clearcompany.com/v1/8a53c87d-2c7f-85ba-20f0-66842148ace3", "", "", "", false},
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
	{"hiringroom embedded vacancy list names the tenant", "https://hiringroom.com/api/vacancies/get_vacancy_list_frame/secco", "hiringroom", "secco", "https://hiringroom.com/api/vacancies/get_vacancy_list_frame/secco", true},
	{"hiringroom embed asset names no tenant", "https://hiringroom.com/assets/js/api/embed.js", "", "", "", false},
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
	{"recruiterbox host is the trakstar tenant", "https://applegreen.recruiterbox.com/jobs/fk0j5vq", "trakstar", "applegreen", "https://applegreen.recruiterbox.com", true},
	{"recruiterbox vendor host is not a tenant", "https://developers.recruiterbox.com/", "", "", "", false},
	{"teamtailor host", "https://bryter.teamtailor.com/jobs/12345-senior-go", "teamtailor", "bryter.teamtailor.com", "https://bryter.teamtailor.com", true},
	{"dvinci posting", "https://cadenas.dvinci-hr.com/de/jobs/20291/junior-genai-engineer-mwd-cadenas-augsburg", "dvinci", "cadenas.dvinci-hr.com", "https://cadenas.dvinci-hr.com", true},
	{"dvinci nested tenant", "https://karriere.hahngruppe.dvinci-hr.com/en/jobs/iframe", "dvinci", "karriere.hahngruppe.dvinci-hr.com", "https://karriere.hahngruppe.dvinci-hr.com", true},
	{"dvinci easy", "https://weig.dvinci-easy.com/jobPublication/list.json", "dvinci", "weig.dvinci-easy.com", "https://weig.dvinci-easy.com", true},
	{"dvinci vendor site not a tenant", "https://www.dvinci-hr.com/", "", "", "", false},
	{"digitalrecruiters vendor-domain site", "https://sennebogen-careers.digitalrecruiters.com/de/annonce/4656321-projektleiter-mwd-94315-straubing", "digitalrecruiters", "sennebogen-careers.digitalrecruiters.com", "https://sennebogen-careers.digitalrecruiters.com", true},
	{"digitalrecruiters marketing host", "https://resources.digitalrecruiters.com/le-recrutement", "", "", "", false},
	{"digitalrecruiters console", "https://app.digitalrecruiters.com/connexion", "", "", "", false},
	{"talentsoft posting", "https://geodis-recrute.talent-soft.com/offre-de-emploi/emploi-agent-d-exploitation-rungis-h-f_18315.aspx?LCID=1040", "talentsoft", "geodis-recrute.talent-soft.com", "https://geodis-recrute.talent-soft.com", true},
	{"talentsoft legacy domain", "https://safran.profils.org/offre-de-emploi/liste-offres.aspx", "talentsoft", "safran.profils.org", "https://safran.profils.org", true},
	{"talentsoft vendor apex not a tenant", "https://www.talent-soft.com/", "", "", "", false},
	{"dvinci third domain", "https://bomag.dvinci.de/de/jobs/123/x", "dvinci", "bomag.dvinci.de", "https://bomag.dvinci.de", true},
	{"dvinci marketing site not a tenant", "https://www.dvinci.de/", "", "", "", false},
	{"rexx posting", "https://karriere-hse24-portal.rexx-systems.com/Head-of-IT-de-j3700.html?sid=97d5fe5688cbbaae18e1e395144534e8", "rexx", "karriere-hse24-portal.rexx-systems.com", "https://karriere-hse24-portal.rexx-systems.com", true},
	{"rexx recruitment listing", "https://horiba-portal.rexx-recruitment.com/stellenangebote.html", "rexx", "horiba-portal.rexx-recruitment.com", "https://horiba-portal.rexx-recruitment.com", true},
	{"rexx asset host not a tenant", "https://cdn.rexx-systems.com/x.css", "", "", "", false},
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
}
