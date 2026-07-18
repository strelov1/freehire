package contribution

import "testing"

// TestRecognizeBoard checks the network-free URL→(source, board, canonical) parse across both
// extraction modes: path (board = first path segment on a fixed host) and subdomain (board =
// leftmost DNS label, canonical collapses to the bare host). A single-tenant/unknown host or a
// board-less URL is declined.
func TestRecognizeBoard(t *testing.T) {
	cases := []struct {
		name          string
		raw           string
		wantSource    string
		wantBoard     string
		wantCanonical string
		wantOK        bool
	}{
		// path
		{"greenhouse vacancy strips utm", "https://job-boards.greenhouse.io/alpaca/jobs/5745893004?utm=x#top", "greenhouse", "alpaca", "https://job-boards.greenhouse.io/alpaca/jobs/5745893004", true},
		{"greenhouse board listing", "https://job-boards.greenhouse.io/alpaca", "greenhouse", "alpaca", "https://job-boards.greenhouse.io/alpaca", true},
		{"lever strips /apply", "https://jobs.lever.co/offchainlabs/52c01c91/apply", "lever", "offchainlabs", "https://jobs.lever.co/offchainlabs/52c01c91", true},
		{"ashby vacancy", "https://jobs.ashbyhq.com/blitzy/a741b4e8-8799", "ashby", "blitzy", "https://jobs.ashbyhq.com/blitzy/a741b4e8-8799", true},
		{"deel path", "https://jobs.deel.com/acme/jobs/123", "deel", "acme", "https://jobs.deel.com/acme/jobs/123", true},
		{"jobvite path", "https://jobs.jobvite.com/acme/job/oABC", "jobvite", "acme", "https://jobs.jobvite.com/acme/job/oABC", true},

		// pathlocale — Rippling: an optional leading xx-XX locale segment is skipped; canonical
		// collapses to the board root so a locale-prefixed vacancy, a bare vacancy, and the
		// listing all map to one board.
		{"rippling locale-prefixed vacancy", "https://ats.rippling.com/en-GB/satomic/jobs/48384892-1b6b?utm=x", "rippling", "satomic", "https://ats.rippling.com/satomic", true},
		{"rippling no locale vacancy", "https://ats.rippling.com/satomic/jobs/34aaf2aa", "rippling", "satomic", "https://ats.rippling.com/satomic", true},
		{"rippling board listing", "https://ats.rippling.com/satomic", "rippling", "satomic", "https://ats.rippling.com/satomic", true},
		{"rippling locale only no tenant", "https://ats.rippling.com/en-GB", "", "", "", false},

		// subdomain — canonical collapses to the bare host
		{"recruitee vacancy strips path", "https://acme.recruitee.com/o/senior-go/apply?utm=x", "recruitee", "acme", "https://acme.recruitee.com", true},
		{"recruitee board listing", "https://acme.recruitee.com", "recruitee", "acme", "https://acme.recruitee.com", true},
		{"bamboohr subdomain", "https://acme.bamboohr.com/careers/42", "bamboohr", "acme", "https://acme.bamboohr.com", true},
		{"personio nested apex subdomain", "https://acme.jobs.personio.com/job/9", "personio", "acme", "https://acme.jobs.personio.com", true},

		// host mode — board is the whole careers host, regional TLD varies
		{"zoho eu vacancy strips encoded path + query", "https://be-exec.zohorecruit.eu/jobs/Careers/73534000009044079/%D0%9F%D1%80%D0%BE?source=CareerSite", "zohorecruit", "be-exec.zohorecruit.eu", "https://be-exec.zohorecruit.eu", true},
		{"zoho com host", "https://kaptiva.zohorecruit.com/jobs/Careers/568", "zohorecruit", "kaptiva.zohorecruit.com", "https://kaptiva.zohorecruit.com", true},
		{"zoho in host", "https://incubyte.zohorecruit.in/jobs/Careers/141", "zohorecruit", "incubyte.zohorecruit.in", "https://incubyte.zohorecruit.in", true},
		{"zoho bare apex not a board", "https://zohorecruit.com/", "", "", "", false},
		{"jazzhr applytojob", "https://acme.applytojob.com/apply/abc", "jazzhr", "acme", "https://acme.applytojob.com", true},
		{"trakstar nested apex", "https://acme.hire.trakstar.com/x", "trakstar", "acme", "https://acme.hire.trakstar.com", true},

		// host+path mode — Workday: board is "<host>/<site>" (site case preserved)
		{"workday vacancy", "https://generalmotors.wd5.myworkdayjobs.com/Careers_GM/job/Austin/Senior-Software-Engineer_JR-202614238", "workday", "generalmotors.wd5.myworkdayjobs.com/Careers_GM", "https://generalmotors.wd5.myworkdayjobs.com/Careers_GM", true},
		{"workday board listing", "https://generalmotors.wd5.myworkdayjobs.com/Careers_GM", "workday", "generalmotors.wd5.myworkdayjobs.com/Careers_GM", "https://generalmotors.wd5.myworkdayjobs.com/Careers_GM", true},
		{"workday other pod", "https://goodyear.wd1.myworkdayjobs.com/goodyearcareers/job/x", "workday", "goodyear.wd1.myworkdayjobs.com/goodyearcareers", "https://goodyear.wd1.myworkdayjobs.com/goodyearcareers", true},

		// declined
		{"workday bare host no site", "https://generalmotors.wd5.myworkdayjobs.com", "", "", "", false},
		{"ashby bare host no board", "https://jobs.ashbyhq.com", "", "", "", false},
		{"recruitee bare apex no tenant", "https://recruitee.com/", "", "", "", false},
		{"personio bare apex no tenant", "https://jobs.personio.com", "", "", "", false},
		{"single-tenant geekjob", "https://geekjob.ru/vacancy/6a1e", "", "", "", false},
		{"unknown host", "https://example.com/careers/1", "", "", "", false},
		{"not http", "ftp://acme.recruitee.com", "", "", "", false},
		{"garbage", "not a url", "", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src, board, canon, ok := RecognizeBoard(c.raw)
			if ok != c.wantOK {
				t.Fatalf("RecognizeBoard(%q) ok = %v, want %v", c.raw, ok, c.wantOK)
			}
			if !ok {
				return
			}
			if src != c.wantSource || board != c.wantBoard || canon != c.wantCanonical {
				t.Errorf("RecognizeBoard(%q) = (%q, %q, %q), want (%q, %q, %q)",
					c.raw, src, board, canon, c.wantSource, c.wantBoard, c.wantCanonical)
			}
		})
	}
}

// TestVacancyAndListingSameBoard proves a vacancy URL and a bare board URL for the same company
// collapse to one board (both modes), so the second (any vacancy on it) is a duplicate.
func TestVacancyAndListingSameBoard(t *testing.T) {
	pairs := [][2]string{
		// path
		{"https://jobs.ashbyhq.com/blitzy/a741b4e8", "https://jobs.ashbyhq.com/blitzy"},
		{"https://job-boards.greenhouse.io/acme/jobs/1?utm=x", "https://job-boards.greenhouse.io/acme"},
		// pathlocale (Rippling): a locale-prefixed vacancy and the bare listing collapse to one board
		{"https://ats.rippling.com/en-GB/satomic/jobs/48384892", "https://ats.rippling.com/satomic"},
		// subdomain
		{"https://acme.recruitee.com/o/senior-go", "https://acme.recruitee.com"},
		{"https://acme.bamboohr.com/careers/42/detail", "https://acme.bamboohr.com/careers/list"},
		// host+path (Workday): a vacancy and the site landing collapse to one board
		{"https://gm.wd5.myworkdayjobs.com/Careers_GM/job/x/Eng_JR-1", "https://gm.wd5.myworkdayjobs.com/Careers_GM"},
	}
	for _, p := range pairs {
		sa, ba, _, oka := RecognizeBoard(p[0])
		sb, bb, _, okb := RecognizeBoard(p[1])
		if !oka || !okb || sa != sb || ba != bb {
			t.Errorf("boards diverged: (%q,%q,%v) vs (%q,%q,%v)", sa, ba, oka, sb, bb, okb)
		}
	}
}
