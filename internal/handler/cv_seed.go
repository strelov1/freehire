package handler

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/experience"
	"github.com/strelov1/freehire/internal/resumeextract"
)

// newWorkHistoryReader builds the bank the CV bootstrap reads, or nil when there are no
// queries to build it over — a nil reader seeds from the structured résumé alone rather
// than failing to seed one at all.
func newWorkHistoryReader(queries *db.Queries) seedBankReader {
	if queries == nil {
		return nil
	}
	return experience.NewStore(experience.NewQueriesRepository(queries))
}

// seedBankReader is the bank read the CV bootstrap needs: jobs and projects kept apart.
type seedBankReader interface {
	SeedHistory(ctx context.Context, userID int64) (experience.SeedHistory, error)
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
	bank   seedBankReader
}

// Structured composes the seed source. Contacts prefer candidate-owned contacts; body
// sections (summary, skills, education, …) come from the current structure only — a
// superseded blob is identity-only. Experience and projects prefer the bank when it
// has rows. See internal/resume/AGENTS.md for the identity table.
func (s bankedSeeder) Structured(ctx context.Context, userID int64) (resumeextract.Structured, bool, error) {
	var st resumeextract.Structured
	haveSource := false
	if s.resume != nil {
		composed, ok, err := s.resume.StructureForSeed(ctx, userID)
		if err != nil {
			return resumeextract.Structured{}, false, err
		}
		if ok {
			st = composed
			haveSource = true
		}
	}

	structureExperience := st.Experience
	structureProjects := st.Projects

	// Prefer the bank for experience and projects once it holds rows of that kind. An
	// empty or unreadable bank falls back to the structure so a pending import does not
	// blank roles the extract already has. Structure experience is never merged into a
	// populated bank — that would resurrect roles the user deleted. The two kinds are
	// judged independently: a bank holding only project-kind rows must still fall back
	// to the structure's own Experience, not blank it.
	st.Experience = nil
	st.Projects = nil
	if s.bank != nil {
		hist, err := s.bank.SeedHistory(ctx, userID)
		if err != nil {
			log.Printf("cv seed work history: user %d: %v", userID, err)
			st.Experience = structureExperience
			st.Projects = structureProjects
		} else {
			if hist.HasJobEmployments {
				st.Experience = hist.Experience
			} else {
				st.Experience = structureExperience
			}
			if hist.HasProjectEmployments {
				st.Projects = hist.Projects
			} else {
				st.Projects = structureProjects
			}
		}
	} else {
		st.Experience = structureExperience
		st.Projects = structureProjects
	}

	return st, haveSource && seedable(st), nil
}

// seedable reports whether a structure carries anything a CV could start from. A structure
// that is empty in every seeded field would produce a blank document, which is what the
// caller gets by not asking to seed at all.
func seedable(st resumeextract.Structured) bool {
	return len(st.Experience) > 0 || len(st.Education) > 0 || len(st.Skills) > 0 ||
		len(st.Languages) > 0 || len(st.Projects) > 0 || len(st.Certifications) > 0 ||
		st.FullName != "" || st.Summary != "" || st.Headline != ""
}
