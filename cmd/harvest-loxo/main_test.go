package main

import "testing"

func TestExtractCandidates(t *testing.T) {
	in := []string{
		"https://fitnext.app.loxo.co/fitnext",                     // agency subdomain
		"https://app.loxo.co/agile-recruiter?location_sort=asc",   // bare host + query
		"https://pod4.app.loxo.co/la-tech",                        // regional pod
		"https://app.loxo.co/job/NDI0NzQtN3RzZTNpa2M2NDJ5emowdg==", // job detail — no slug, skip
		"https://fitnext.app.loxo.co/login",                       // non-careers, skip
		"https://app.loxo.co/agile-recruiter",                     // dup of #2 (query stripped)
		"https://example.com/careers",                             // not loxo, skip
		"https://notapp.loxo.co/evil",                             // lookalike host, skip
		"  ",                                                      // blank, skip
	}
	got := extractCandidates(in)
	want := []candidate{
		{"fitnext.app.loxo.co", "fitnext"},
		{"app.loxo.co", "agile-recruiter"},
		{"pod4.app.loxo.co", "la-tech"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d candidates, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("candidate[%d] = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestAgencyName(t *testing.T) {
	cases := map[string]string{
		`<title>Job Listing | FitNext Co.</title>`: "FitNext Co.",
		`<title>Job Listing | LA-Tech</title>`:     "LA-Tech",
		`<title>Something else</title>`:            "", // no agency → caller falls back to slug
	}
	for html, want := range cases {
		if got := agencyName(html); got != want {
			t.Errorf("agencyName(%q) = %q, want %q", html, got, want)
		}
	}
}

func TestIsTechCategory(t *testing.T) {
	if !isTechCategory("backend") {
		t.Error("backend should be tech")
	}
	for _, nonTech := range []string{"marketing", "sales", "support", "management", ""} {
		if isTechCategory(nonTech) {
			t.Errorf("%q should not count as tech", nonTech)
		}
	}
}
