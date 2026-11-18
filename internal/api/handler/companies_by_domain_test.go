package handler

import "testing"

// The same cases run against public.company_site_host in the integration test.
var siteHostCases = []struct{ in, want string }{
	{"stripe.com", "stripe.com"},
	{"https://www.acme.io/careers", "acme.io"},
	{"HTTP://WWW.Acme.IO", "acme.io"},
	{"acme.io:8080/x?y=1#z", "acme.io"},
	{"https://user:pw@acme.io:443/", "acme.io"},
	{"acme.io.", "acme.io"},
	{" www.acme.io/ ", "acme.io"},
	{"\tacme.io\r\n", "acme.io"},
	{"https://jobs.acme.io", "jobs.acme.io"},
	{"https://acme.io?ref=1", "acme.io"},
	{"", ""},
	{"   ", ""},
}

func TestNormalizeSiteHost(t *testing.T) {
	for _, c := range siteHostCases {
		if got := normalizeSiteHost(c.in); got != c.want {
			t.Errorf("normalizeSiteHost(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
