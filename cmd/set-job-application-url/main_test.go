package main

import "testing"

func TestApplicationURL(t *testing.T) {
	for _, tc := range []struct {
		url          string
		clear, valid bool
	}{
		{"https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735?src=LinkedIn", false, true},
		{"http://careers.example.org/jobs/1#apply", false, true},
		{"", true, true},
		{"", false, false},
		{"https://example.org/1", true, false},
		{"javascript:alert(1)", false, false},
		{"//example.org/1", false, false},
		{"https://user:password@example.org/1", false, false},
		{"https://example.org/\n1", false, false},
		{"https:///jobs/1", false, false},
	} {
		_, err := applicationURL(tc.url, tc.clear)
		if (err == nil) != tc.valid {
			t.Errorf("applicationURL(%q, %v): %v", tc.url, tc.clear, err)
		}
	}
}
