package companyname

import "testing"

func TestBoardFromURL(t *testing.T) {
	cases := []struct {
		source string
		url    string
		want   string
		wantOK bool
	}{
		{"pinpoint", "https://lbresearch.pinpointhq.com/en/postings/78ba", "lbresearch", true},
		{"bamboohr", "https://321theagency.bamboohr.com/careers/42", "321theagency", true},
		{"lever", "https://jobs.lever.co/1inch/abc-123", "1inch", true},
		{"ashby", "https://jobs.ashbyhq.com/0x/some-id", "0x", true},
		{"greenhouse", "https://boards.greenhouse.io/acme/jobs/42", "acme", true},
		{"greenhouse", "https://job-boards.greenhouse.io/acme/jobs/42", "acme", true},
		// Unknown source or unparseable URL yields no board.
		{"unknown-ats", "https://example.com/x", "", false},
		{"pinpoint", "not a url", "", false},
		{"lever", "https://jobs.lever.co/", "", false},
	}
	for _, c := range cases {
		got, ok := BoardFromURL(c.source, c.url)
		if got != c.want || ok != c.wantOK {
			t.Errorf("BoardFromURL(%q, %q) = (%q, %v), want (%q, %v)",
				c.source, c.url, got, ok, c.want, c.wantOK)
		}
	}
}
