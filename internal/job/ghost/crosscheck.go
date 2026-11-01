package ghost

import (
	"strings"
	"unicode"

	"github.com/strelov1/freehire/internal/job/jobhash"
)

// The gates a batch must pass before any posting in it is stamped. Measured on prod dry
// runs (2026-11-01, 1.74M open aggregator postings): with every gate below in place the
// run would stamp 2,978 postings, against 87,120 before any of them; the per-reason
// skip counts in the report are each gate's share.
const (
	// minCorroborated is how many of a batch's postings the board must carry before
	// the rest may be called absent from it. A board that carries none of what a
	// source lists for the company is not evidence about that source's postings:
	// it is another tenant behind the same slug, a regional board of a company with
	// several, or a source that rewrites every title. Without this gate 69% of the
	// would-be stamps (21,055 of 30,356) sat in such batches; with the others in
	// place it still declines 8,563.
	minCorroborated = 1
	// maxAbsentShare is the largest share of a batch that may be absent. Above it
	// absence is the rule for that company and source rather than the exception —
	// an agency advertising clients' roles, or a board we only partly crawl — and an
	// individual posting's absence says nothing. It declines 4,126, and is what took
	// the staffing agencies (square-one-resources, itfs, manpower, tek-systems,
	// adecco) off the top of the list.
	maxAbsentShare = 0.5
	// minBoardTitles is the smallest board treated as the company's catalogue. A
	// board of one or two titles is as often a stray tenant sharing the slug or a
	// job-centre listing as the employer's own careers page.
	minBoardTitles = 5
	// minRecruiterPostings is how many of a company's open postings must say they
	// are published on a client's behalf before the company is treated as an
	// intermediary. Two rather than one, so a single employer posting that mentions
	// "our client" in passing does not cost the company its verdict.
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

// Skips counts the postings a cross-check declined to judge, by reason. Each reason
// is reported on its own because they mean different things to the person reading
// the calibration report: NoBoard and Uncorroborated are our crawl coverage,
// MostlyAbsent is a company whose aggregator footprint is not its board's, and the
// rest are postings that can never be evidence however good the coverage gets.
type Skips struct {
	// NoCompany: the posting names no employer. An empty slug is not a company, and
	// grouping every such posting as one would check them all against whatever
	// boards also lack a company — measured on prod (2026-10-31) that made the
	// empty slug the single largest "company" in the stamp list, at 10,559.
	NoCompany int
	// ViaRecruiter: the posting says it is an intermediary advertising a client's
	// role. Such a role is legitimately absent from the intermediary's own board —
	// the failure mode this signal is known for.
	ViaRecruiter int
	// RecruiterCompany: a posting of a company that has stated, in other postings,
	// that it publishes clients' roles (see minRecruiterPostings). An agency's
	// postings advertise clients' roles whether or not each one says so.
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

// CrosscheckResult is the writes one company's cross-check implies, plus how many
// postings it declined to judge. Skips are reported rather than swallowed: a run
// that silently judged nothing looks identical to one that judged everything
// present, and those two mean opposite things.
type CrosscheckResult struct {
	Stamp []int64
	Clear []int64
	Skips Skips
}

// Crosscheck decides, for one company's postings from ONE aggregator source, which to
// stamp as absent from the company's own board and which to clear.
//
// An empty (or near-empty, see minBoardTitles) board is the coverage gate: we do not
// crawl this company's board, so nothing here is evidence and every posting is skipped. Treating an empty board as
// "absent from everywhere" would report our own coverage gaps as the employer's
// fault — which is how the previous attempt at this feature failed, by measuring our
// data instead of the world.
//
// A board we crawl is not yet a board that covers what this source lists, so two
// more gates judge the batch as a whole before any posting in it is stamped
// (minCorroborated, maxAbsentShare). The batch is one source's postings because
// sources differ in how faithfully they carry an employer's title: a source that
// rewrites every title fails corroboration for that company without costing the
// company its verdict from a source that copies titles verbatim.
//
// The stamps are the latest run's verdict and nothing else: every stamped posting
// this run does not re-stamp — present, unjudged, or judged by a gate — is cleared.
// A stamp left in place on a posting the current rules no longer accuse would go on
// accusing until it aged out, and a rule tightened after a stamp was written is
// exactly the case where that is wrong.
//
// A stamp that is still absent is RE-stamped rather than left alone: the reader
// ignores a stamp that has aged out, so refreshing it is what keeps a true absence
// true. A posting already unstamped and not stamped now produces no write.
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
			// No usable title means no key, and a posting that cannot match also
			// cannot be judged absent. Stamping it would make every untitled
			// posting evidence against its employer.
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

// clearAll withdraws the stamps of postings a gate declined to judge and returns
// nothing left to stamp.
func clearAll(out *CrosscheckResult, postings []Posting) []Posting {
	for _, p := range postings {
		if p.Stamped {
			out.Clear = append(out.Clear, p.ID)
		}
	}
	return nil
}

// boardIndex answers whether a company's board carries a role, matching the ways the
// same role is retitled between an employer's board and an aggregator. Measured on a
// 35-posting sample of prod's stamps (2026-11-01), exact title keys left most of them
// false: an aggregator drops a dash or a gender marker, inflects a word ("Operadora
// de Caixa" against the board's "Operador(a) de Caixa"), or adds a pitch to the
// title ("Data Engineer: Build Data Lakehouse & AI Pipelines" against "Data Engineer
// (m/w/d) 100%"). So a posting is carried when its key equals a board title's
// (jobhash.TitleKeys) or when the two titles share most of their words (overlaps).
//
// Every choice here leans toward "carried": a match too generous costs one ghost we
// miss, a match too strict accuses the employer of a role it is advertising.
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
	// minOverlapWords is the fewest significant words a title needs before word
	// overlap may match it. One: a one-word title is then carried only by a title
	// containing that word, so the board's "Hôte - H/F" carries the aggregator's
	// "Hôte - H/F ANNIVERSAIRES / ANIMATIONS" — and its "Developer" carries every
	// developer role, the generous direction this index is built to err in.
	minOverlapWords = 1
	// stemRunes truncates every word to its first runes, a crude stem that is
	// enough to meet an inflection or a plural: "operadora"/"operador",
	// "services"/"service", "repositora"/"repositor".
	stemRunes = 5
	// minWordRunes drops shorter words: articles and prepositions ("de", "of",
	// "la"), gender markers once their slashes are folded ("m w d", "h f",
	// "k m n"), and a stray letter. They are what two unrelated titles share most.
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

// titleWords is a title's significant words, stemmed: lower-cased and
// punctuation-folded (jobhash.TitleKeys), digits-only words and words shorter than
// minWordRunes dropped, each cut to stemRunes.
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
