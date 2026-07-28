package handler

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/experience"
	"github.com/strelov1/freehire/internal/resumeextract"
)

// newWorkHistoryReader builds the bank the CV bootstrap reads, or nil when there are no
// queries to build it over — a nil reader seeds a CV with no work history rather than
// failing to seed one at all.
func newWorkHistoryReader(queries *db.Queries) workHistoryReader {
	if queries == nil {
		return nil
	}
	return experience.NewStore(experience.NewQueriesRepository(queries))
}

// workHistoryReader is the bank read the CV bootstrap needs.
type workHistoryReader interface {
	WorkHistory(ctx context.Context, userID int64) ([]resumeextract.Experience, error)
}

// bankedSeeder answers "what should a new CV start from" with the candidate's banked work
// history plus the sections the stored structure still owns. It satisfies cv.Seeder, so
// internal/cv needs no knowledge of the bank at all: it keeps receiving a Structured and
// keeps mapping it field by field.
//
// This is the step that closes the loop. Evidence a candidate confirmed while tailoring
// for one vacancy lands in the base CV they create next — which is the difference between
// the bank being the agent's memory and it being the user's.
type bankedSeeder struct {
	resume structuredResumeReader
	bank   workHistoryReader
}

// Structured composes the seed source. The bool reports whether there is anything to seed
// FROM: an empty bank and no structure means a caller asking to bootstrap a CV has nothing
// to build one out of, which the tailoring path turns into "add a résumé first".
func (s bankedSeeder) Structured(ctx context.Context, userID int64) (resumeextract.Structured, bool, error) {
	var st resumeextract.Structured
	if s.resume != nil {
		if stored, ok, err := s.resume.Structured(ctx, userID); err == nil && ok {
			st = stored
		}
	}

	// The structure's own copy of the work history is replaced, never merged: the bank is
	// where experience lives now, and merging would resurrect roles the user deleted.
	st.Experience = nil
	if s.bank != nil {
		history, err := s.bank.WorkHistory(ctx, userID)
		if err != nil {
			log.Printf("cv seed work history: user %d: %v", userID, err)
		} else {
			st.Experience = history
		}
	}
	return st, seedable(st), nil
}

// seedable reports whether a structure carries anything a CV could start from. A structure
// that is empty in every seeded field would produce a blank document, which is what the
// caller gets by not asking to seed at all.
func seedable(st resumeextract.Structured) bool {
	return len(st.Experience) > 0 || len(st.Education) > 0 || len(st.Skills) > 0 ||
		len(st.Languages) > 0 || st.FullName != "" || st.Summary != "" || st.Headline != ""
}
