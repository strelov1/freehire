package main

import (
	"context"
	"testing"
)

func TestManatalProbe(t *testing.T) {
	p := manatalProber{}
	listing := `<html><head><title> - Aisling Group | Career Page</title></head><body>
<a href="/aisling-group/job/38R3R8">Recruiter</a>
<a href="/aisling-group/job/38R3R8">Apply</a>
<a href="/aisling-group/job/534YR3">Analyst</a>
<a href="/aisling-group">Home</a>
</body></html>`
	getter := fakeGetter{
		"https://careers-page.com/aisling-group": listing,
		"https://careers-page.com/empty": `<html><head><title> - Nobody Ltd | Career Page</title></head>
<body><a href="/empty">Home</a></body></html>`,
		"https://careers-page.com/untitled": `<html><body>
<a href="/untitled/job/1">Role</a></body></html>`,
	}
	// The tenant name is the board owner and is what the entry must be labelled with: on an
	// agency board it is the agency, never the client the posting is for.
	if name, n, err := p.probe(context.Background(), getter, "aisling-group"); err != nil || name != "Aisling Group" || n != 2 {
		t.Errorf("live: got (%q,%d,%v), want (\"Aisling Group\",2,nil)", name, n, err)
	}
	// A board with a name but no postings is not live, and must not be proposed.
	if _, n, err := p.probe(context.Background(), getter, "empty"); err != nil || n != 0 {
		t.Errorf("empty: got n=%d err=%v, want 0,nil", n, err)
	}
	// No parseable title: still live, but the name is left to the caller's fallback rather
	// than invented.
	if name, n, err := p.probe(context.Background(), getter, "untitled"); err != nil || name != "" || n != 1 {
		t.Errorf("untitled: got (%q,%d,%v), want (\"\",1,nil)", name, n, err)
	}
	if _, n, err := p.probe(context.Background(), getter, "gone"); err != nil || n != 0 {
		t.Errorf("gone: got n=%d err=%v, want 0,nil", n, err)
	}
}

func TestManatalTenantName(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{" - Aisling Group | Career Page", "Aisling Group"},
		{"Aisling Group | Career Page", "Aisling Group"},
		{" - PT Yamaha Motor Indonesia | Career Page", "PT Yamaha Motor Indonesia"},
		// A tenant whose own name carries a pipe must not be truncated at the first one.
		{" - Saje | Retail | Career Page", "Saje | Retail"},
		{"Career Page", ""},
		{"", ""},
	} {
		if got := manatalTenantName(tc.in); got != tc.want {
			t.Errorf("manatalTenantName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
