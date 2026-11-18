package ghost

import (
	"strings"
	"unicode"

	"github.com/strelov1/freehire/internal/job/jobhash"
)

// The gates a batch must pass before any posting in it is stamped.
const (
	// minCorroborated is how many of a batch's postings the board must carry before the
	// rest may be called absent. A board carrying none of a source's postings is not
	// evidence: it may be another tenant behind the slug, a regional board, or a source
	// that rewrites every title.
	minCorroborated = 1
	// maxAbsentShare is the largest share of a batch that may be absent. Above it absence
	// is the rule for that company and source (an agency, or a partly crawled board), so
	// one posting's absence says nothing.
	maxAbsentShare = 0.5
	// minBoardTitles is the smallest board treated as the company's catalogue; a tiny one
	// is as often a stray tenant or job-centre listing as the careers page.
	minBoardTitles = 5
	// minRecruiterPostings is how many open postings must say they are published on a
	// client's behalf before the company counts as an intermediary. Two, so one passing
	// "our client" mention does not cost the company its verdict.
	minRecruiterPostings = 2
)

// Board is what we crawl of one company's OWN hiring surface.
type Board struct {
	// Titles are the company's open titles from sources of kind `ats` or `company`.
	Titles []string
	// RecruiterPostings counts the company's open postings, from any source, that
	// state they are published on a client's behalf (jobs.via_recruiter).
	RecruiterPostings int64
}

// Posting is one open aggregator posting under cross-check. Stamped is whether it
// already carries an absence stamp, which decides whether a verdict needs a write.
// ViaRecruiter is the posting's own statement that it is published on a client's
// behalf (jobs.via_recruiter).
type Posting struct {
	ID           int64
	CompanySlug  string
	Title        string
	Stamped      bool
	ViaRecruiter bool
}

// Skips counts the postings a cross-check declined to judge, by reason. NoBoard and
// Uncorroborated are our crawl coverage, MostlyAbsent is a company whose aggregator
// footprint is not its board's, and the rest can never be evidence.
type Skips struct {
	// NoCompany: the posting names no employer. An empty slug is not a company, and
	// grouping them would check all against whatever boards also lack one.
	NoCompany int
	// ViaRecruiter: the posting says it advertises a client's role, which is legitimately
	// absent from the intermediary's own board.
	ViaRecruiter int
	// RecruiterCompany: a posting of a company that says in other postings that it
	// publishes clients' roles (see minRecruiterPostings).
	RecruiterCompany int
	// NoBoard: we crawl no board of this company's (the coverage gate), or one too
	// small to stand for its catalogue (see minBoardTitles).
	NoBoard int
	// NoTitle: the title normalizes to nothing, so it has no key to match.
	NoTitle int
	// Uncorroborated: an absent posting in a batch none of whose postings the board
	// carries (see minCorroborated).
	Uncorroborated int
	// MostlyAbsent: an absent posting in a batch where absence is the rule rather
	// than the exception (see maxAbsentShare).
	MostlyAbsent int
}

// Add accumulates another result's skips, reason by reason.
func (s *Skips) Add(o Skips) {
	s.NoCompany += o.NoCompany
	s.ViaRecruiter += o.ViaRecruiter
	s.RecruiterCompany += o.RecruiterCompany
	s.NoBoard += o.NoBoard
	s.NoTitle += o.NoTitle
	s.Uncorroborated += o.Uncorroborated
	s.MostlyAbsent += o.MostlyAbsent
}

// Total is every skipped posting, whatever the reason.
func (s Skips) Total() int {
	return s.NoCompany + s.ViaRecruiter + s.RecruiterCompany + s.NoBoard + s.NoTitle +
		s.Uncorroborated + s.MostlyAbsent
}

// CrosscheckResult is the writes one company's cross-check implies, plus how many postings
// it declined to judge. Skips are reported so a run that judged nothing is distinguishable
// from one that found everything present.
type CrosscheckResult struct {
	Stamp []int64
	Clear []int64
	Skips Skips
}

// Crosscheck decides, for one company's postings from ONE aggregator source, which to
// stamp as absent from the company's own board and which to clear.
//
// An empty or near-empty board (see minBoardTitles) is the coverage gate: we do not crawl
// this company's board, so nothing is evidence and every posting is skipped. Treating it
// as "absent from everywhere" would blame the employer for our coverage gaps.
//
// Two more gates (minCorroborated, maxAbsentShare) judge the batch as a whole before any
// posting is stamped. A batch is one source's postings because sources differ in how
// faithfully they carry titles; a title-rewriting source must not cost the company its
// verdict from a verbatim one.
//
// Stamps are the latest run's verdict only: every stamped posting this run does not
// re-stamp (present, unjudged, or gated) is cleared, so a rule tightened after a stamp was
// written stops accusing at once.
//
// A still-absent stamp is re-stamped, because the reader ignores an aged-out stamp.
// A posting unstamped and not stamped now produces no write.
func Crosscheck(postings []Posting, b Board) CrosscheckResult {
	var out CrosscheckResult
	board := newBoardIndex(b.Titles)
	intermediary := b.RecruiterPostings >= minRecruiterPostings

	var absent []Posting
	present := 0
	for _, p := range postings {
		switch {
		case p.CompanySlug == "":
			out.Skips.NoCompany++
		case p.ViaRecruiter:
			out.Skips.ViaRecruiter++
		case len(b.Titles) < minBoardTitles:
			out.Skips.NoBoard++
		case intermediary:
			out.Skips.RecruiterCompany++
		case len(jobhash.TitleKeys(p.Title)) == 0:
			// No key means it cannot match, so it cannot be judged absent either.
			out.Skips.NoTitle++
		case board.carries(p.Title):
			present++
		default:
			absent = append(absent, p)
			continue
		}
		if p.Stamped {
			out.Clear = append(out.Clear, p.ID)
		}
	}

	judged := present + len(absent)
	switch {
	case len(absent) == 0:
	case present < minCorroborated:
		out.Skips.Uncorroborated += len(absent)
		absent = clearAll(&out, absent)
	case float64(len(absent)) > maxAbsentShare*float64(judged):
		out.Skips.MostlyAbsent += len(absent)
		absent = clearAll(&out, absent)
	}
	for _, p := range absent {
		out.Stamp = append(out.Stamp, p.ID)
	}
	return out
}

// clearAll withdraws the stamps of postings a gate declined to judge and returns nil.
func clearAll(out *CrosscheckResult, postings []Posting) []Posting {
	for _, p := range postings {
		if p.Stamped {
			out.Clear = append(out.Clear, p.ID)
		}
	}
	return nil
}

// boardIndex answers whether a company's board carries a role, allowing for how an
// aggregator retitles it: dropped dashes or gender markers, inflected words ("Operadora" vs
// "Operador(a)"), an added pitch. A posting is carried when its key equals a board title's
// (jobhash.TitleKeys) or the two titles share most of their words (overlaps).
//
// Every choice leans toward "carried": too generous misses one ghost, too strict accuses
// the employer of a role it is advertising.
type boardIndex struct {
	keys  map[string]struct{}
	words []map[string]struct{}
}

func newBoardIndex(titles []string) boardIndex {
	b := boardIndex{keys: make(map[string]struct{}, 2*len(titles))}
	for _, title := range titles {
		for _, key := range jobhash.TitleKeys(title) {
			b.keys[key] = struct{}{}
		}
		if words := titleWords(title); len(words) >= minOverlapWords {
			b.words = append(b.words, words)
		}
	}
	return b
}

func (b boardIndex) carries(title string) bool {
	for _, key := range jobhash.TitleKeys(title) {
		if _, ok := b.keys[key]; ok {
			return true
		}
	}
	words := titleWords(title)
	if len(words) < minOverlapWords {
		return false
	}
	for _, onBoard := range b.words {
		if overlaps(words, onBoard) {
			return true
		}
	}
	return false
}

const (
	// minOverlapWords is the fewest significant words a title needs before word overlap
	// may match it. At one, a board's "Developer" carries every developer role, the
	// generous direction this index errs in.
	minOverlapWords = 1
	// stemRunes truncates every word to its first runes, a crude stem that meets an
	// inflection or plural ("operadora"/"operador", "services"/"service").
	stemRunes = 5
	// minWordRunes drops shorter words (articles, folded gender markers like "m w d"),
	// which are what unrelated titles share most.
	minWordRunes = 3
)

// overlaps reports whether two titles share at least two thirds of the words of the
// shorter one — "Support System Administrator" and "Service Desk/System
// Administrator" are one role, "Go Developer" and "Rust Developer" are not.
func overlaps(a, b map[string]struct{}) bool {
	small, big := a, b
	if len(small) > len(big) {
		small, big = big, small
	}
	shared := 0
	for w := range small {
		if _, ok := big[w]; ok {
			shared++
		}
	}
	return 3*shared >= 2*len(small)
}

// titleWords is a title's significant words, stemmed: folded via jobhash.TitleKeys, with
// digits-only and short words dropped, each cut to stemRunes.
func titleWords(title string) map[string]struct{} {
	keys := jobhash.TitleKeys(title)
	if len(keys) == 0 {
		return nil
	}
	fields := strings.Fields(keys[0])
	set := make(map[string]struct{}, len(fields))
	for _, w := range fields {
		r := []rune(w)
		if len(r) < minWordRunes || allDigits(r) {
			continue
		}
		if len(r) > stemRunes {
			r = r[:stemRunes]
		}
		set[string(r)] = struct{}{}
	}
	return set
}

func allDigits(r []rune) bool {
	for _, c := range r {
		if !unicode.IsDigit(c) {
			return false
		}
	}
	return true
}
