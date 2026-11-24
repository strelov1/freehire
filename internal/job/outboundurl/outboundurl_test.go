package outboundurl

import "testing"

func TestTagEncodedSource(t *testing.T) {
	got := Tag("https://example.com/?z=1&utm%5Fsource=indeed&a=x%2Fy")
	want := "https://example.com/?z=1&a=x%2Fy&utm_source=freehire.me"
	if got != want {
		t.Fatalf("Tag() = %q, want %q", got, want)
	}
}

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

func TestParseSrc(t *testing.T) {
	for _, s := range []Src{SrcWeb, SrcMCP, SrcOJCP, SrcAssistant, SrcEmail, SrcTelegram, SrcAPI} {
		if got := ParseSrc(string(s)); got != s {
			t.Errorf("ParseSrc(%q) = %q", s, got)
		}
	}
	for _, in := range []string{"", "WEB", "other", "x&y=1", "https://evil.example"} {
		if got := ParseSrc(in); got != SrcOther {
			t.Errorf("ParseSrc(%q) = %q, want other", in, got)
		}
	}
}

func TestApplyURL(t *testing.T) {
	if got, want := ApplyURL("acme-go-engineer-1a2b3c4d", SrcMCP), "https://freehire.me/api/v1/go/acme-go-engineer-1a2b3c4d?src=mcp"; got != want {
		t.Errorf("ApplyURL = %q, want %q", got, want)
	}
}
