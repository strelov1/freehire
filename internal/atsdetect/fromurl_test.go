package atsdetect

import (
	"testing"

	"github.com/strelov1/freehire/internal/atsboard"
)

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
		{
			name:     "paycom",
			url:      "https://www.paycomonline.net/v4/ats/web.php/portal/3b4555a93baac45919556b5f901f7b83/jobs/383852?utm_source=Role",
			provider: "paycom", board: "3b4555a93baac45919556b5f901f7b83", ok: true,
		},
		// iCIMS: board is the tenant in careers-<board>.icims.com.
		{
			name:     "icims careers prefix",
			url:      "https://careers-hcsgcorp.icims.com/jobs/704470/dietary-aide/job",
			provider: "icims", board: "hcsgcorp", ok: true,
		},
		{
			name: "icims subdomain without careers prefix is not harvestable",
			url:  "https://uk-external-novelis.icims.com/jobs/49037/mechanical-day-technician",
			ok:   false,
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
		// Shapes whose URL does not yield the board id our adapter expects.
		{
			name: "adp vanity not cid:ccId",
			url:  "https://myjobs.adp.com/afcindustries/cx/job-details?reqid=5001150982906",
			ok:   false,
		},
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

// TestLocalShapesStayOutsideTheSharedTable guards the split that remains.
//
// FromURL delegates to atsboard.Recognize and keeps only the five shapes atsboard deliberately
// excludes. That exclusion is load-bearing, not an oversight: atsboard is the accept-set for
// internal/contribution, which PAYS for onboarded boards, so moving one of these in is a
// money-affecting decision that needs its own proposal — not a tidy-up.
//
// So the two sets must stay disjoint in both directions. If a shape here starts being recognised
// by the shared table, this package's case for it is dead code silently shadowed by the
// delegation above it; and the widening happened without anyone arguing for it.
func TestLocalShapesStayOutsideTheSharedTable(t *testing.T) {
	local := map[string]string{
		"icims":  "https://careers-acme.icims.com/jobs/1234/engineer/job",
		"oracle": "https://eeho.fa.us2.oraclecloud.com/hcmUI/CandidateExperience/en/sites/CX_1/job/1234",
		"taleo":  "https://valero.taleo.net/careersection/2/jobsearch.ftl",
		"neogov": "https://www.governmentjobs.com/careers/cityofboise/jobs/4567",
		"paycom": "https://www.paycomonline.net/v4/ats/web.php/portal/0123456789abcdef0123456789abcdef/jobs",
		"comeet": "https://www.comeet.com/jobs/overwolf/B1.001",
	}
	for provider, rawurl := range local {
		t.Run(provider, func(t *testing.T) {
			if src, board, _, ok := atsboard.Recognize(rawurl); ok {
				t.Errorf("atsboard now recognises the %s shape as (%q, %q). Either this package's "+
					"case is dead code shadowed by the delegation, or the contribution accept-set "+
					"widened without a proposal — decide which, do not just delete one.",
					provider, src, board)
			}
			got, _, ok := FromURL(rawurl)
			if !ok || got != provider {
				t.Errorf("FromURL(%q) = (%q, ok=%v), want %q — this package still owns the shape",
					rawurl, got, ok, provider)
			}
		})
	}
}
