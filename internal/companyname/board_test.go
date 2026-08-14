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

func TestBoard(t *testing.T) {
	cases := []struct {
		source, name, url string
		want              string
		wantOK            bool
	}{
		// join: the board is the numeric placeholder name itself — its job URL
		// carries the company's slug domain, not the numeric id, so it can't be
		// recovered from the URL at all.
		{"join", "175014", "https://join.com/companies/goodweekcom/16575727-x", "175014", true},
		// A join company whose name already got resolved has nothing to backfill.
		{"join", "Goodweek", "https://join.com/companies/goodweekcom/16575727-x", "", false},
		// Non-join sources are untouched — still fall through to BoardFromURL.
		{"pinpoint", "lbresearch", "https://lbresearch.pinpointhq.com/en/postings/78ba", "lbresearch", true},
	}
	for _, c := range cases {
		got, ok := Board(c.source, c.name, c.url)
		if got != c.want || ok != c.wantOK {
			t.Errorf("Board(%q, %q, %q) = (%q, %v), want (%q, %v)",
				c.source, c.name, c.url, got, ok, c.want, c.wantOK)
		}
	}
}
