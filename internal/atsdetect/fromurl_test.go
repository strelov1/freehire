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
