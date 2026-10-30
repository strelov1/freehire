package sources

import (
	"context"
	"testing"
)

// Workday's remoteType is a tenant-configured label, not an enum. Every label below is one a
// live tenant publishes (sampled across ~1,000 boards, 2026-10-30); the last group is labels
// that state no arrangement this vocabulary can hold, or offer two, and must stay empty.
func TestWorkdayWorkMode(t *testing.T) {
	cases := map[string]string{
		"Remote": "remote", "Fully Remote": "remote", "Fully remote": "remote",
		"Remote-Restricted": "remote", "Remote - Flexible": "remote", "Remote - Nationwide": "remote",
		"Remote - U.S.": "remote", "Remote - Region Specific": "remote",
		"Remote - Must reside within territory": "remote", "Home Based/Remote": "remote",
		"Remote work only": "remote", "Virtual": "remote",

		"Hybrid": "hybrid", "Hybrid Working": "hybrid", "Hybrid Work": "hybrid", "Hybrid Remote": "hybrid",
		"Hybrid - depending upon institutional need":         "hybrid",
		"Hybrid - Onsite / 2 Days Remote":                    "hybrid",
		"Hybrid (Onsite/WFA)":                                "hybrid",
		"This position is a hybrid of office/remote working": "hybrid",
		"Onsite - Hybrid Eligible":                           "hybrid",
		"Eligible for Hybrid Work":                           "hybrid",
		"Partially Remote":                                   "hybrid",
		"Office Flex":                                        "hybrid",
		"Location Type: Hybrid Option Available (Mix of in-person and remote)": "hybrid",
		"Mobiles Arbeiten möglich": "hybrid",
		"Partial mobile working":   "hybrid",
		"Télétravail partiel":      "hybrid",
		"Télétravail possible":     "hybrid",
		"Híbrido":                  "hybrid",

		"On-site": "onsite", "On-Site": "onsite", "Onsite": "onsite", "On Site": "onsite", "On site": "onsite",
		"On Campus": "onsite", "Office": "onsite", "In Office": "onsite", "In-Office": "onsite",
		"In-Office Position": "onsite", "In Person": "onsite", "In-Person": "onsite",
		"Fully Onsite": "onsite", "Fully In-Person": "onsite", "Onsite Only": "onsite", "Onsite Job": "onsite",
		"On-Site/In-Office": "onsite", "Full-Time Onsite": "onsite", "In Office 100%": "onsite",
		"n/a - In Office Only": "onsite", "In Branch": "onsite", "Not Remote": "onsite",
		"On-site only, not eligible for remote work":      "onsite",
		"Location Type: In Person (Remote Not Available)": "onsite",
		"Not Eligible for Hybrid Work":                    "onsite",
		"Not Eligible for Agile Working":                  "onsite",
		"No mobile working":                               "onsite",
		"Mobiles Arbeiten nicht möglich":                  "onsite",
		"Poste uniquement en présentiel":                  "onsite",
		"Travail sur site":                                "onsite",

		"": "", "Not specified": "", "Modalités de télétravail non renseignées": "",
		"Flex": "", "Fully Flexible": "", "Hybrid or Onsite": "", "Remote working allowed": "",
		"On Site - 4 Days": "", "Field Based": "", "Field-Based": "", "Field": "", "Field Remote": "",
		"On-Road": "", "Client site": "", "Frontline": "",
		"Growth": "", "Mid-Market": "", "College of the Holy Cross": "",
	}
	for label, want := range cases {
		if got := workdayWorkMode(label); got != want {
			t.Errorf("workdayWorkMode(%q) = %q, want %q", label, got, want)
		}
	}
}

// A remoteType that DENIES remote work must not raise the remote flag: the adapter used to set
// it on any label containing "remote", so "On-site only, not eligible for remote work" (a live
// label) read as remote.
func TestWorkdayDeniedRemoteIsNotRemote(t *testing.T) {
	fake := (&routedHTTP{}).
		route("/Careers/jobs", `{"total": 1, "jobPostings": [
			{"title": "Teller", "externalPath": "/job/Ohio/Teller_JR-9", "locationsText": "Columbus, OH"}
		]}`).
		route("Teller_JR-9", `{"jobPostingInfo": {"title": "Teller", "jobDescription": "<p>x</p>",
			"location": "Columbus, OH", "remoteType": "On-site only, not eligible for remote work"}}`)

	jobs, err := NewWorkday(fake).Fetch(context.Background(), CompanyEntry{
		Company: "Acme", Provider: "workday", Board: "acme.wd1.myworkdayjobs.com/Careers",
	})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("Fetch = %v, %v", jobs, err)
	}
	if jobs[0].Remote || jobs[0].WorkMode != "onsite" {
		t.Errorf("Remote/WorkMode = %v/%q, want false/onsite", jobs[0].Remote, jobs[0].WorkMode)
	}
}
