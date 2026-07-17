package contribution

import "testing"

// TestRecognizeBoard checks the network-free URL→(source, board, canonical) parse: a
// supported multi-tenant ATS link — vacancy OR bare board listing — yields the company
// board with tails stripped; a single-tenant/unknown host or a board-less URL is declined.
func TestRecognizeBoard(t *testing.T) {
	cases := []struct {
		name          string
		raw           string
		wantSource    string
		wantBoard     string
		wantCanonical string
		wantOK        bool
	}{
		{"greenhouse vacancy strips utm", "https://job-boards.greenhouse.io/alpaca/jobs/5745893004?utm=x#top", "greenhouse", "alpaca", "https://job-boards.greenhouse.io/alpaca/jobs/5745893004", true},
		{"greenhouse board listing", "https://job-boards.greenhouse.io/alpaca", "greenhouse", "alpaca", "https://job-boards.greenhouse.io/alpaca", true},
		{"greenhouse eu subdomain", "https://boards.eu.greenhouse.io/acme/jobs/1", "greenhouse", "acme", "https://boards.eu.greenhouse.io/acme/jobs/1", true},
		{"lever strips /apply", "https://jobs.lever.co/offchainlabs/52c01c91/apply", "lever", "offchainlabs", "https://jobs.lever.co/offchainlabs/52c01c91", true},
		{"ashby vacancy", "https://jobs.ashbyhq.com/blitzy/a741b4e8-8799-4539-b1c2-78d69ff625e7", "ashby", "blitzy", "https://jobs.ashbyhq.com/blitzy/a741b4e8-8799-4539-b1c2-78d69ff625e7", true},
		{"ashby board listing trailing slash", "https://jobs.ashbyhq.com/blitzy/", "ashby", "blitzy", "https://jobs.ashbyhq.com/blitzy", true},
		{"workable board listing", "https://apply.workable.com/acme", "workable", "acme", "https://apply.workable.com/acme", true},

		{"ashby bare host no board", "https://jobs.ashbyhq.com", "", "", "", false},
		{"greenhouse bare host no board", "https://job-boards.greenhouse.io/", "", "", "", false},
		{"single-tenant geekjob", "https://geekjob.ru/vacancy/6a1ebb85", "", "", "", false},
		{"unknown host", "https://example.com/careers/1", "", "", "", false},
		{"not http", "ftp://jobs.ashbyhq.com/blitzy", "", "", "", false},
		{"garbage", "not a url", "", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src, board, canon, ok := recognizeBoard(c.raw)
			if ok != c.wantOK {
				t.Fatalf("recognizeBoard(%q) ok = %v, want %v", c.raw, ok, c.wantOK)
			}
			if !ok {
				return
			}
			if src != c.wantSource || board != c.wantBoard || canon != c.wantCanonical {
				t.Errorf("recognizeBoard(%q) = (%q, %q, %q), want (%q, %q, %q)",
					c.raw, src, board, canon, c.wantSource, c.wantBoard, c.wantCanonical)
			}
		})
	}
}

// TestVacancyAndListingSameBoard proves a vacancy URL and a bare board URL for the same
// company collapse to one board, so the second (any vacancy on it) is a duplicate.
func TestVacancyAndListingSameBoard(t *testing.T) {
	pairs := [][2]string{
		{"https://jobs.ashbyhq.com/blitzy/a741b4e8", "https://jobs.ashbyhq.com/blitzy"},
		{"https://jobs.ashbyhq.com/blitzy/a1c86055", "https://jobs.ashbyhq.com/blitzy/"},
		{"https://job-boards.greenhouse.io/acme/jobs/1?utm=x", "https://job-boards.greenhouse.io/acme"},
	}
	for _, p := range pairs {
		sa, ba, _, oka := recognizeBoard(p[0])
		sb, bb, _, okb := recognizeBoard(p[1])
		if !oka || !okb || sa != sb || ba != bb {
			t.Errorf("boards diverged: (%q,%q,%v) vs (%q,%q,%v)", sa, ba, oka, sb, bb, okb)
		}
	}
}
