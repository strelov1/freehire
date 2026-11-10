package atsboard

// recognizeCases7 is part 7 of 7 of the TestRecognize table, in order.
var recognizeCases7 = []recognizeCase{
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
	{"polymer posting", "https://jobs.polymer.co/motive/41656", "polymer", "motive", "https://jobs.polymer.co/motive/41656", true},
	{"polymer board", "https://jobs.polymer.co/rumor-avenue-131c654d-d4de-4007-82af-1e959a296220", "polymer", "rumor-avenue-131c654d-d4de-4007-82af-1e959a296220", "https://jobs.polymer.co/rumor-avenue-131c654d-d4de-4007-82af-1e959a296220", true},
	{"flatchr vacancy", "https://careers.flatchr.io/fr/company/medialex/vacancy/jkj0mpzeoow9xngx-manager-commercial-h-f/", "flatchr", "medialex", "https://careers.flatchr.io/fr/company/medialex", true},
	{"flatchr board in english", "https://careers.flatchr.io/en/company/levertech-2", "flatchr", "levertech-2", "https://careers.flatchr.io/fr/company/levertech-2", true},
	{"flatchr board document", "https://careers.flatchr.io/company/mila.json", "flatchr", "mila", "https://careers.flatchr.io/fr/company/mila", true},
	{"flatchr site page is not a board", "https://careers.flatchr.io/fr/404-not-found", "", "", "", false},
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
