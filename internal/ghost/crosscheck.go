package ghost

import "github.com/strelov1/freehire/internal/jobhash"

// Posting is one open aggregator posting under cross-check. Stamped is whether it
// already carries an absence stamp, which decides whether a verdict needs a write.
type Posting struct {
	ID          int64
	CompanySlug string
	Title       string
	Stamped     bool
}

// CrosscheckResult is the writes one company's cross-check implies, plus how many
// postings it declined to judge. Skipped is reported rather than swallowed: a run
// that silently judged nothing looks identical to one that judged everything
// present, and those two mean opposite things.
type CrosscheckResult struct {
	Stamp   []int64
	Clear   []int64
	Skipped int
}

// Crosscheck decides, for one company's aggregator postings, which to stamp as absent
// from the company's own board and which to clear.
//
// boardTitles is that company's open titles from sources of kind `ats` or `company`.
// An EMPTY boardTitles is the coverage gate: we do not crawl this company's board, so
// nothing here is evidence and every posting is skipped. Treating an empty board as
// "absent from everywhere" would report our own coverage gaps as the employer's
// fault — which is how the previous attempt at this feature failed, by measuring our
// data instead of the world.
//
// A stamp that is still absent is RE-stamped rather than left alone: the reader
// ignores a stamp that has aged out, so refreshing it is what keeps a true absence
// true. A posting already correct in the database produces no write at all.
func Crosscheck(postings []Posting, boardTitles []string) CrosscheckResult {
	var out CrosscheckResult
	if len(boardTitles) == 0 {
		out.Skipped = len(postings)
		return out
	}

	// The company is fixed for this batch, so keys are scoped by title alone —
	// passing the slug on both sides would only add a constant prefix.
	onBoard := make(map[string]struct{}, len(boardTitles))
	for _, title := range boardTitles {
		if key := jobhash.RoleKey("", title); key != "" {
			onBoard[key] = struct{}{}
		}
	}

	for _, p := range postings {
		key := jobhash.RoleKey("", p.Title)
		if key == "" {
			// No usable title means no key, and a posting that cannot match also
			// cannot be judged absent. Stamping it would make every untitled
			// posting evidence against its employer.
			out.Skipped++
			continue
		}
		if _, present := onBoard[key]; present {
			if p.Stamped {
				out.Clear = append(out.Clear, p.ID)
			}
			continue
		}
		out.Stamp = append(out.Stamp, p.ID)
	}
	return out
}
