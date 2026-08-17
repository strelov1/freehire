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
		{"talenthr vacancy", "https://jobs.talenthr.io/dnext/senior-backend-developer-2/22", "talenthr", "dnext", "https://jobs.talenthr.io/dnext/senior-backend-developer-2/22", true},
		{"deel path", "https://jobs.deel.com/acme/jobs/123", "deel", "acme", "https://jobs.deel.com/acme/jobs/123", true},
		{"jobvite path", "https://jobs.jobvite.com/acme/job/oABC", "jobvite", "acme", "https://jobs.jobvite.com/acme/job/oABC", true},

		// Reserved segments — a platform path word is never a tenant. Jobvite serves the same
		// board bare and behind a "careers" portal segment, so the first segment read the portal
		// word as the board and onboarded nothing; Greenhouse's embed machinery has no board in
		// the path at all (the slug is in the `for=` param, which atsdetect reads).
		{"jobvite portal segment skipped", "https://jobs.jobvite.com/careers/ness/jobs", "jobvite", "ness", "https://jobs.jobvite.com/careers/ness/jobs", true},
		{"greenhouse embed app has no board", "https://job-boards.greenhouse.io/embed/job_app?token=1", "", "", "", false},
		{"greenhouse embed script has no board", "https://boards.greenhouse.io/embed/job_board/js?for=acme", "", "", "", false},

		// Manatal's hosted career-page domain is path-based, not subdomain-based, and its boards
		// live in manatal.yml — careerspage.yml is deliberately empty.
		{"manatal careers-page posting", "https://www.careers-page.com/nearshore-business-solutions/job/5W939XW3", "manatal", "nearshore-business-solutions", "https://www.careers-page.com/nearshore-business-solutions/job/5W939XW3", true},
		{"manatal careers-page listing", "https://www.careers-page.com/hiretidal", "manatal", "hiretidal", "https://www.careers-page.com/hiretidal", true},

		// pathprefix — the ATS's OWN API host. A careers page on the employer's domain loads its
		// listing over XHR, so the board is named in that API URL and nowhere else in the page.
		{"ashby posting API", "https://api.ashbyhq.com/posting-api/job-board/phantom?includeCompensation=false", "ashby", "phantom", "https://api.ashbyhq.com/posting-api/job-board/phantom", true},
		{"greenhouse boards API", "https://boards-api.greenhouse.io/v1/boards/anthropic/jobs", "greenhouse", "anthropic", "https://boards-api.greenhouse.io/v1/boards/anthropic", true},
		{"lever postings API", "https://api.lever.co/v0/postings/matchgroup?mode=json", "lever", "matchgroup", "https://api.lever.co/v0/postings/matchgroup", true},
		{"api host without a board", "https://api.ashbyhq.com/posting-api/job-board", "", "", "", false},
		{"api host off-prefix path", "https://api.lever.co/v1/something/else", "", "", "", false},

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

		// pathlocale — Rippling: an optional leading xx-XX locale segment is skipped; canonical
		// collapses to the board root so a locale-prefixed vacancy, a bare vacancy, and the
		// listing all map to one board.
		{"rippling locale-prefixed vacancy", "https://ats.rippling.com/en-GB/satomic/jobs/48384892-1b6b?utm=x", "rippling", "satomic", "https://ats.rippling.com/satomic", true},
		{"rippling no locale vacancy", "https://ats.rippling.com/satomic/jobs/34aaf2aa", "rippling", "satomic", "https://ats.rippling.com/satomic", true},
		{"rippling board listing", "https://ats.rippling.com/satomic", "rippling", "satomic", "https://ats.rippling.com/satomic", true},
		{"rippling locale only no tenant", "https://ats.rippling.com/en-GB", "", "", "", false},

		// subdomain — canonical collapses to the bare host
		{"recruitee vacancy strips path", "https://acme.recruitee.com/o/senior-go/apply?utm=x", "recruitee", "acme", "https://acme.recruitee.com", true},
		{"recruitee board listing", "https://acme.recruitee.com", "recruitee", "acme", "https://acme.recruitee.com", true},
		{"bamboohr subdomain", "https://acme.bamboohr.com/careers/42", "bamboohr", "acme", "https://acme.bamboohr.com", true},
		{"personio nested apex subdomain", "https://acme.jobs.personio.com/job/9", "personio", "acme", "https://acme.jobs.personio.com", true},
		{"personio de host", "https://reflex-aerospace-gmbh.jobs.personio.de/job/2679152?display=en#apply", "personio", "reflex-aerospace-gmbh", "https://reflex-aerospace-gmbh.jobs.personio.de", true},
		{"softgarden subdomain", "https://moll.softgarden.io/job/123/apply", "softgarden", "moll", "https://moll.softgarden.io", true},
		// softgarden also serves tenants under a regional career host, <tenant>.career.softgarden.de.
		// The tenant label is the same board the adapter fetches at <board>.softgarden.io (verified
		// live: agilitaschweiz answers on both), so only the apex differs. Keyed on softgarden.io
		// alone, the .de host left the label behind a second DNS label and was declined.
		{"softgarden regional career host", "https://agilitaschweiz.career.softgarden.de/jobs/65679426/sap-consultant/", "softgarden", "agilitaschweiz", "https://agilitaschweiz.career.softgarden.de", true},
		{"hibob careers subdomain", "https://qogita.careers.hibob.com/jobs/ceb6c947-c906-44d1-a56b-bb33ae5599fa", "hibob", "qogita", "https://qogita.careers.hibob.com", true},
		{"hibob apply tail collapses to the same board", "https://unique.careers.hibob.com/jobs/f8d9a0bc/apply", "hibob", "unique", "https://unique.careers.hibob.com", true},

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
		{"trakstar nested apex", "https://acme.hire.trakstar.com/x", "trakstar", "acme", "https://acme.hire.trakstar.com", true},
		{"teamtailor host", "https://bryter.teamtailor.com/jobs/12345-senior-go", "teamtailor", "bryter.teamtailor.com", "https://bryter.teamtailor.com", true},
		{"factorial host it", "https://muffin.factorial.it/job/1", "factorial", "muffin.factorial.it", "https://muffin.factorial.it", true},
		{"factorialhr base-domain variant", "https://9net.factorialhr.com.br/job/2", "factorial", "9net.factorialhr.com.br", "https://9net.factorialhr.com.br", true},

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
	}
	for _, c := range cases {
		src, _, _, ok := Recognize(c.raw)
		if !ok || src != c.wantSource {
			t.Errorf("Recognize(%q) source = %q (ok %v), want %q", c.raw, src, ok, c.wantSource)
		}
	}
}
