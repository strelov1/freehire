package outboundurl

import "testing"

func TestTag(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "no query string appends with ?",
			in:   "https://ats.example.com/job/123",
			want: "https://ats.example.com/job/123?utm_source=freehire.me",
		},
		{
			name: "existing query appends with &",
			in:   "https://ats.example.com/job?id=123",
			want: "https://ats.example.com/job?id=123&utm_source=freehire.me",
		},
		{
			name: "existing utm_source is overwritten",
			in:   "https://ats.example.com/job?utm_source=indeed",
			want: "https://ats.example.com/job?utm_source=freehire.me",
		},
		{
			name: "fragment is preserved after the query",
			in:   "https://ats.example.com/job#apply",
			want: "https://ats.example.com/job?utm_source=freehire.me#apply",
		},
		{
			// freehire#4003: ESS opens the posting only from this exact query. Re-encoding it
			// (idp=3419120%2F) loads an empty page, and sorting would reorder the source's own.
			name: "existing query bytes and order are kept verbatim",
			in:   "https://www.ess.gov.si/iskanje-dela/?idp=3419120/#/pdm/3419120",
			want: "https://www.ess.gov.si/iskanje-dela/?idp=3419120/&utm_source=freehire.me#/pdm/3419120",
		},
		{
			name: "parameter order is not sorted",
			in:   "https://ats.example.com/job?z=1&a=2",
			want: "https://ats.example.com/job?z=1&a=2&utm_source=freehire.me",
		},
		{
			name: "every published utm_source is replaced, wherever it sits",
			in:   "https://ats.example.com/job?utm_source=indeed&id=7&utm_source=x",
			want: "https://ats.example.com/job?id=7&utm_source=freehire.me",
		},
		{
			name: "empty url is returned unchanged",
			in:   "",
			want: "",
		},
		{
			name: "unparseable url is returned unchanged",
			in:   "https://ats.example.com/%zz",
			want: "https://ats.example.com/%zz",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Tag(tt.in); got != tt.want {
				t.Errorf("Tag(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestUntag(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"round-trips the ESS query verbatim", Tag("https://www.ess.gov.si/iskanje-dela/?idp=3419120/#/pdm/3419120"), "https://www.ess.gov.si/iskanje-dela/?idp=3419120/#/pdm/3419120"},
		{"drops the query when ours was the only parameter", "https://ats.example.com/job?utm_source=freehire.me", "https://ats.example.com/job"},
		{"leaves a source's own utm_source alone", "https://ats.example.com/job?utm_source=indeed", "https://ats.example.com/job?utm_source=indeed"},
		{"keeps other parameters in order", "https://ats.example.com/job?z=1&utm_source=freehire.me&a=2", "https://ats.example.com/job?z=1&a=2"},
		{"empty url is returned unchanged", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Untag(tt.in); got != tt.want {
				t.Errorf("Untag(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
