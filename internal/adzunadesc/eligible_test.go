package adzunadesc

import "testing"

func TestEligible(t *testing.T) {
	cases := []struct {
		name   string
		source string
		url    string
		want   bool
	}{
		{
			name:   "gb hosted details page",
			source: "adzuna",
			url:    "https://www.adzuna.co.uk/jobs/details/5834162887?utm_medium=api&utm_source=freehire.me",
			want:   true,
		},
		{
			name:   "au hosted details page, no /jobs/ segment",
			source: "adzuna",
			url:    "https://www.adzuna.com.au/details/5834030919?utm_medium=api&utm_source=freehire.me",
			want:   true,
		},
		{
			name:   "gb ad-network tracking redirect",
			source: "adzuna",
			url:    "https://www.adzuna.co.uk/jobs/land/ad/5834043525?se=mtYwRYiT8RGWPoJqQXFPnw&utm_medium=api&utm_source=freehire.me&v=97EBE9964C19BD64F75B0D5F0F51CDAF10E7056A",
			want:   false,
		},
		{
			name:   "de ad-network tracking redirect",
			source: "adzuna",
			url:    "https://www.adzuna.de/land/ad/5834070034?se=6B4dRYiT8RG9ks23jnlnsA&utm_medium=api&utm_source=freehire.me&v=6E9F8034E47A436F6959A798B38831DAFEFDEBE7",
			want:   false,
		},
		{
			name:   "not adzuna at all",
			source: "greenhouse",
			url:    "https://www.adzuna.co.uk/jobs/details/5834162887",
			want:   false,
		},
		{
			name:   "empty url",
			source: "adzuna",
			url:    "",
			want:   false,
		},
		{
			name:   "unparseable url",
			source: "adzuna",
			url:    "://not a url",
			want:   false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Eligible(c.source, c.url); got != c.want {
				t.Errorf("Eligible(%q, %q) = %v, want %v", c.source, c.url, got, c.want)
			}
		})
	}
}
