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
		// Unknown source (greenhouse job URLs are vanity domains — no board) or
		// unparseable URL yields no board.
		{"greenhouse", "https://a16z.com/about/jobs/?gh_jid=42", "", false},
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
