package telegram

import (
	"fmt"
	"strings"
)

// ExtractedJob is one vacancy extracted from a Telegram post. Company and
// location are optional — many posts state neither. Salary, contacts, and other
// details stay inside Description: enrichment derives structure from it later,
// exactly as for ATS-ingested jobs.
type ExtractedJob struct {
	Title       string `json:"title"`
	Company     string `json:"company,omitempty"`
	Location    string `json:"location,omitempty"`
	Remote      bool   `json:"remote,omitempty"`
	Description string `json:"description"`
}

// Extraction is the typed result of classifying + extracting one post. Zero jobs
// is a normal outcome: the post was not a vacancy.
type Extraction struct {
	Jobs []ExtractedJob `json:"jobs"`
}

// Validate drops any malformed job — missing title or description — from the
// extraction and keeps the rest, mutating e.Jobs in place. The LLM is not trusted
// to be correct, but a KindAuthored post routinely bundles several roles in one
// message, and Store.Complete already tolerates skipping a single bad draft; an
// all-or-nothing error here would dead-letter the whole post's well-formed jobs
// along with the one malformed one. It returns an error only when every job in
// the extraction was malformed, since a post that named zero jobs to begin with
// is itself the normal "not a vacancy" outcome.
func (e *Extraction) Validate() error {
	if len(e.Jobs) == 0 {
		return nil
	}
	kept := e.Jobs[:0]
	var firstErr error
	for i, j := range e.Jobs {
		switch {
		case strings.TrimSpace(j.Title) == "":
			if firstErr == nil {
				firstErr = fmt.Errorf("telegram: extracted job %d has empty title", i)
			}
		case strings.TrimSpace(j.Description) == "":
			if firstErr == nil {
				firstErr = fmt.Errorf("telegram: extracted job %d (%s) has empty description", i, j.Title)
			}
		default:
			kept = append(kept, j)
		}
	}
	e.Jobs = kept
	if len(kept) == 0 {
		return firstErr
	}
	return nil
}
