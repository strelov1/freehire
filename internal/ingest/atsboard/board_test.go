package atsboard

import (
	"slices"
	"testing"
)

type recognizeCase struct {
	name          string
	raw           string
	wantSource    string
	wantBoard     string
	wantCanonical string
	wantOK        bool
}

// TestRecognizeBoard checks the network-free URL→(source, board, canonical) parse across both
// extraction modes: path (board = first path segment on a fixed host) and subdomain (board =
// leftmost DNS label, canonical collapses to the bare host). A single-tenant/unknown host or a
// board-less URL is declined.
func TestRecognize(t *testing.T) {
	cases := slices.Concat(recognizeCases1, recognizeCases2, recognizeCases3, recognizeCases4, recognizeCases5, recognizeCases6, recognizeCases7)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src, board, canon, ok := Recognize(c.raw)
			if ok != c.wantOK {
				t.Fatalf("Recognize(%q) ok = %v, want %v", c.raw, ok, c.wantOK)
			}
			if !ok {
				return
			}
			if src != c.wantSource || board != c.wantBoard || canon != c.wantCanonical {
				t.Errorf("Recognize(%q) = (%q, %q, %q), want (%q, %q, %q)",
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
		// pathlocalepair (Dayforce): a culture-prefixed vacancy and the bare site landing
		{"https://jobs.dayforcehcm.com/en-US/dcrusa/join-us/jobs/12345", "https://jobs.dayforcehcm.com/dcrusa/join-us"},
		// query (Paycor): a posting and the board's career home, which differ in path AND query
		{"https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=4028f88b24c330a20124eb33b4ad1653&id=8a7883a6", "https://recruitingbypaycor.com/career/CareerHome.action?clientId=4028f88b24c330a20124eb33b4ad1653"},
		// hosttenant (UKG Ready): a vacancy and its board's career page
		{"https://secure.entertimeonline.com/ta/10284.careers?ShowJob=123456", "https://secure.entertimeonline.com/ta/10284.careers"},
		// host (HiringThing) and subdomain (HRM Direct): a posting and the board root
		{"https://4mindsai.hiringthing.com/job/12345/senior-engineer", "https://4mindsai.hiringthing.com/"},
		{"https://123loadboard.hrmdirect.com/employment/job-opening.php?req=3456&req_loc=1029", "https://123loadboard.hrmdirect.com/employment/job-openings.php?search=true"},
		{"https://apply.clearcompany.com/bkbehavior?jobId=3522713d-0ff6-5e08-72c3-617411eaccb5", "https://apply.clearcompany.com/bkbehavior"},
		{"https://senca.hireclick.com/jb/family-development-associate-nebraska-city/view/264587", "https://senca.hireclick.com/jobboard/jobboard.aspx"},
		// path (Gusto): a paged listing and the bare board
		{"https://jobs.gusto.com/boards/100-degrees-consulting-ec8f0e9c-d6b4-4a32-8544-9c213ca6bc90?page=3", "https://jobs.gusto.com/boards/100-degrees-consulting-ec8f0e9c-d6b4-4a32-8544-9c213ca6bc90"},
		// pathpattern (NEOGOV, Paycom): a posting and the board's own listing
		{"https://www.governmentjobs.com/careers/champlin/jobs/5179173/analyst", "https://www.governmentjobs.com/careers/champlin"},
		{"https://www.paycomonline.net/v4/ats/web.php/portal/0c46774eb61f343c911be787715ae6f2/jobs/61570", "https://www.paycomonline.net/v4/ats/web.php/portal/0c46774eb61f343c911be787715ae6f2/career-page"},
		// query with a tenant host (Eightfold): a posting and the career site
		{"https://eaton.eightfold.ai/careers/job/1?domain=eaton.com", "https://eaton.eightfold.ai/careers?domain=eaton.com"},
	}
	for _, p := range pairs {
		sa, ba, _, oka := Recognize(p[0])
		sb, bb, _, okb := Recognize(p[1])
		if !oka || !okb || sa != sb || ba != bb {
			t.Errorf("boards diverged: (%q,%q,%v) vs (%q,%q,%v)", sa, ba, oka, sb, bb, okb)
		}
	}
}

// TestRecognizeMapsHostsToTheIngestProviderName pins that a recognised host resolves to the
// source key the CATALOGUE uses, not to a name derived from the domain. Getting this wrong is
// silently expensive: the board-tracked check looks jobs up by (source, board), so a board we
// already crawl under one name looks brand new under another — it is recorded as a fresh
// contribution and paid for, and board coverage finds no ingest adapter to read it with.
//
// Factorial is the case in point: one adapter serves <tenant>.factorial.<tld> and
// <tenant>.factorialhr.<tld>, and it reports "factorial" for both.
func TestRecognizeMapsHostsToTheIngestProviderName(t *testing.T) {
	cases := []struct{ raw, wantSource string }{
		{"https://muffin.factorial.it/job/1", "factorial"},
		{"https://9net.factorialhr.com.br/job/2", "factorial"},
		{"https://4farma.factorialhr.pt/job/3", "factorial"},
		{"https://etrade.gr8people.com/jobs/4709", "gr8people"},
		{"https://batesville.workgr8.com/jobs/1146", "gr8people"},
	}
	for _, c := range cases {
		src, _, _, ok := Recognize(c.raw)
		if !ok || src != c.wantSource {
			t.Errorf("Recognize(%q) source = %q (ok %v), want %q", c.raw, src, ok, c.wantSource)
		}
	}
}

// TestQueryModeRowsAreConfigured guards the modes whose rows are not self-contained: a query
// entry needs a second row in queryBoards naming the parameter, a querypair entry a row in
// queryPairBoards naming both, a keyedsegment entry a row in keyedSegmentBoards naming the key
// and the board shape, and a pathpattern entry a row in pathPatternBoards whose pattern captures
// exactly one group. Without it the host matches and then declines every link on it —
// a platform that looks supported and recognises nothing. The rest of the table keeps the
// "one row plus one test case" promise on its own.
func TestQueryModeRowsAreConfigured(t *testing.T) {
	for _, a := range atsBoards {
		switch a.mode {
		case modeQuery:
			if q, ok := queryBoards[a.host]; !ok || q.param == "" || q.listingPath == "" || q.boardPattern == nil {
				t.Errorf("atsBoards entry %q (%s) is %s mode but queryBoards has no complete entry for it",
					a.host, a.source, modeQuery)
			}
		case modeQueryPair:
			q, ok := queryPairBoards[a.host]
			if !ok || q.first == "" || q.second == "" || q.listingPath == "" ||
				q.firstPattern == nil || q.secondPattern == nil || q.foldFirst == nil {
				t.Errorf("atsBoards entry %q (%s) is %s mode but queryPairBoards has no complete entry for it",
					a.host, a.source, modeQueryPair)
			}
		case modeKeyedSegment:
			if k, ok := keyedSegmentBoards[a.host]; !ok || k.key == "" || k.boardPattern == nil {
				t.Errorf("atsBoards entry %q (%s) is %s mode but keyedSegmentBoards has no complete entry for it",
					a.host, a.source, modeKeyedSegment)
			}
		case modePathPattern:
			p, ok := pathPatternBoards[a.host]
			if !ok || p.path == nil || p.path.NumSubexp() != 1 || p.listing == "" || (p.param != "") != (p.paramPattern != nil) {
				t.Errorf("atsBoards entry %q (%s) is %s mode but pathPatternBoards has no complete entry for it",
					a.host, a.source, modePathPattern)
			}
		}
	}
}

// A host whose mode this package has not been taught to read must be DECLINED, not read by
// whichever rule happens to sit last. The first-path-segment rule used to be that fallback,
// so a platform added with a new mode — one whose board is a query parameter, a host label,
// or anything else — would have had the vendor's own leading path segment taken as a tenant:
// a board that does not exist, which the contribution flow records and pays for.
func TestRecognizeDeclinesAModeItCannotRead(t *testing.T) {
	original := atsBoards
	t.Cleanup(func() { atsBoards = original })
	atsBoards = append(append([]struct{ host, source, mode string }(nil), original...),
		struct{ host, source, mode string }{"jobs.notyetsupported.test", "notyetsupported", "amodenobodywrote"})

	raw := "https://jobs.notyetsupported.test/careers/acme/job/1"
	if src, board, canonical, ok := Recognize(raw); ok {
		t.Errorf("Recognize(%q) = (%q, %q, %q, ok=true), want declined — no rule reads this mode",
			raw, src, board, canonical)
	}
}

// TestRecognizeReturnsTheBoardTheBoardFileStores pins the extraction of the six platforms added
// most recently against boards copied VERBATIM out of the shipped sources/<provider>.yml, one
// per board-id shape each file contains.
//
// A rule that disagrees with the board file is worse than no rule at all. The board string is
// what the contribution flow records and pays for, and what board coverage hands to the ingest
// adapter — so a near-miss files an employer under a board no crawl can resolve, while looking
// exactly like a successful recognition. The board files are the only authority on the shape: a
// provider's crawl is built around it.
func TestRecognizeReturnsTheBoardTheBoardFileStores(t *testing.T) {
	cases := []struct{ name, raw, source, board string }{
		// paycor: the 32-hex clientId, lower-case in the file (the platform serves either).
		{"paycor Day Kimball Health", "https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=4028f88b24c330a20124eb33b4ad1653&id=1", "paycor", "4028f88b24c330a20124eb33b4ad1653"},
		{"paycor Clark Hill", "https://recruitingbypaycor.com/career/CareerHome.action?clientId=8a29a018478a87e70147a7588a6343cf", "paycor", "8a29a018478a87e70147a7588a6343cf"},

		// dayforce: "<tenant>/<site>", the two segments after the culture.
		{"dayforce 4Refuel", "https://jobs.dayforcehcm.com/en-US/4refuel/candidateportal", "dayforce", "4refuel/candidateportal"},
		{"dayforce 99 Cents Only Stores", "https://jobs.dayforcehcm.com/en-US/99cents/candidateportal/jobs/1", "dayforce", "99cents/candidateportal"},
		{"dayforce DCR USA", "https://jobs.dayforcehcm.com/dcrusa/join-us", "dayforce", "dcrusa/join-us"},

		// gusto: the whole "<company-slug>-<company-uuid>" segment.
		{"gusto 100 Degrees Consulting", "https://jobs.gusto.com/boards/100-degrees-consulting-ec8f0e9c-d6b4-4a32-8544-9c213ca6bc90", "gusto", "100-degrees-consulting-ec8f0e9c-d6b4-4a32-8544-9c213ca6bc90"},
		{"gusto 121G", "https://jobs.gusto.com/boards/121g-consulting-llc-f41b3691-0c93-4406-acb7-6b28c2c69601?page=1", "gusto", "121g-consulting-llc-f41b3691-0c93-4406-acb7-6b28c2c69601"},

		// hiringthing: the full careers host, on the vendor's domain and on three resellers'.
		{"hiringthing 4Minds", "https://4mindsai.hiringthing.com/job/1", "hiringthing", "4mindsai.hiringthing.com"},
		{"hiringthing 10 Federal", "https://10federal-partners-inc.prismhr-hire.com/job/1", "hiringthing", "10federal-partners-inc.prismhr-hire.com"},
		{"hiringthing Integral Senior Living", "https://1440-by-the-bay.oasisrecruit.com/job/1", "hiringthing", "1440-by-the-bay.oasisrecruit.com"},
		{"hiringthing BASH", "https://1235-keystone-way-llc.gnahiring.com/job/1", "hiringthing", "1235-keystone-way-llc.gnahiring.com"},

		// ukgready: "<host>/<tenant>", one per host family in the file (including the bare apex
		// and the alphanumeric tenant ids, which are not all numeric).
		{"ukgready Rapid Robert's", "https://secure.entertimeonline.com/ta/10284.careers?ShowJob=1", "ukgready", "secure.entertimeonline.com/10284"},
		{"ukgready Phoenix Zoo", "https://secure7.saashr.com/ta/6010265.careers", "ukgready", "secure7.saashr.com/6010265"},
		{"ukgready TotalTec", "https://saashr.com/ta/6199997.careers", "ukgready", "saashr.com/6199997"},
		{"ukgready Copper State Bolt & Nut", "https://secure3.yourpayrollhr.com/ta/ihr0568.careers", "ukgready", "secure3.yourpayrollhr.com/ihr0568"},
		{"ukgready SPC Global", "https://secure.workforceready.com.au/ta/6162382.careers", "ukgready", "secure.workforceready.com.au/6162382"},
		{"ukgready C. Caswell Engineering", "https://secure.workforceready.eu/ta/6121944.careers", "ukgready", "secure.workforceready.eu/6121944"},
		{"ukgready General Health System", "https://prd01-hcm01.npr.mykronos.com/ta/6042637.careers", "ukgready", "prd01-hcm01.npr.mykronos.com/6042637"},

		// hrmdirect: the tenant subdomain alone.
		{"hrmdirect 123Loadboard", "https://123loadboard.hrmdirect.com/employment/job-opening.php?req=1&req_loc=2", "hrmdirect", "123loadboard"},
		{"hrmdirect 1A Auto", "https://1aautoinc.hrmdirect.com/employment/job-openings.php?search=true", "hrmdirect", "1aautoinc"},
		// ...and the same slug on ClearCompany, which now serves every HRM Direct tenant.
		{"hrmdirect on clearcompany SMS", "https://apply.clearcompany.com/sms?jobId=3522713d-0ff6-5e08-72c3-617411eaccb5", "hrmdirect", "sms"},
		{"hrmdirect on clearcompany both hosts agree", "https://apply.clearcompany.com/123loadboard", "hrmdirect", "123loadboard"},

		// hireclick: the tenant subdomain alone.
		{"hireclick Southwest Truck & RV", "https://southwesttruckandrv.hireclick.com/jb/professional-diesel-technician-phoenix-az/view/267596", "hireclick", "southwesttruckandrv"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src, board, _, ok := Recognize(c.raw)
			if !ok || src != c.source || board != c.board {
				t.Errorf("Recognize(%q) = (%q, %q, ok=%v), want (%q, %q) — the board sources/%s.yml stores",
					c.raw, src, board, ok, c.source, c.board, c.source)
			}
		})
	}
}
