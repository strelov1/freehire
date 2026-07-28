package handler

import (
	"context"
	"log"
	"strings"

	"github.com/strelov1/freehire/internal/experience"
	"github.com/strelov1/freehire/internal/resumeextract"
)

// experienceBank is the one operation the résumé upload path needs from the experience
// bank. Narrow on purpose: the upload feeds the bank and never reads it, so nothing else
// belongs behind this seam.
type experienceBank interface {
	Import(ctx context.Context, userID int64, entries []experience.ImportEntry, sourceRef string) (experience.ImportResult, error)
}

// currentEndLabels are the end-date labels a CV uses for a role that has not ended. A CV
// states "Present", not a flag, so the current role has to be read off the label — and an
// empty label is the other way of saying the same thing.
var currentEndLabels = map[string]bool{"": true, "present": true, "current": true, "now": true, "ongoing": true}

// importExperience banks the work history of a freshly-extracted résumé. Best-effort in
// the same way as every other step of the derive path: an unconfigured bank, an
// extraction with no work history, or a failing import is logged (never the CV text) and
// swallowed, leaving the upload and the structured résumé untouched.
func (h *resumeHandlers) importExperience(ctx context.Context, userID int64, st resumeextract.Structured, sourceRef string) {
	if h.bank == nil {
		return
	}
	entries := importEntriesFromStructured(st)
	if len(entries) == 0 {
		return
	}
	result, err := h.bank.Import(ctx, userID, entries, sourceRef)
	if err != nil {
		log.Printf("experience import: user %d: %v", userID, err)
		return
	}
	log.Printf("experience import: user %d: %d employments created, %d filled, %d atoms created, %d already banked",
		userID, result.EmploymentsCreated, result.EmploymentsFilled, result.AtomsCreated, result.AtomsSkipped)
}

// importEntriesFromStructured maps a parsed résumé onto the bank's import shape. This is
// the seam that keeps internal/experience free of any one importer's vocabulary: roles
// and portfolio projects are both places where evidence was produced, and their
// highlights are the evidence.
//
// Nothing from the contact block crosses over. The bank holds what the candidate did; who
// they are stays on the résumé record, where the contact whitelist already governs it.
func importEntriesFromStructured(st resumeextract.Structured) []experience.ImportEntry {
	entries := make([]experience.ImportEntry, 0, len(st.Experience)+len(st.Projects))

	for _, role := range st.Experience {
		entries = append(entries, experience.ImportEntry{
			Employment: experience.Employment{
				Kind:     experience.KindJob,
				Company:  role.Company,
				Role:     role.Title,
				Location: role.Location,
				Start:    role.Start,
				End:      role.End,
				Current:  currentEndLabels[strings.ToLower(strings.TrimSpace(role.End))],
				Summary:  role.Summary,
				Stack:    role.Stack,
			},
			Claims: role.Highlights,
		})
	}

	// A portfolio project is a place too: it is named, it produced achievements, and a
	// tailored CV draws on it the same way it draws on a job.
	for _, project := range st.Projects {
		entries = append(entries, experience.ImportEntry{
			Employment: experience.Employment{
				Kind:    experience.KindProject,
				Company: project.Name,
			},
			Claims: project.Highlights,
		})
	}

	return entries
}
