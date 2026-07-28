package experience

import (
	"context"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/resumeextract"
)

// This file composes the de-identified candidate profile — the one thing the fit chain
// scores and the one thing an API-key caller reads. It is the second adapter that names a
// concrete source shape (see import_resume.go); Store and Retrieve still know nothing
// about résumés.

// Professional composes the owner's de-identified candidate profile: work history from the
// bank, everything else from the structured résumé.
func (s *Store) Professional(ctx context.Context, userID int64, st resumeextract.Structured) (resumeextract.Professional, error) {
	history, err := s.WorkHistory(ctx, userID)
	if err != nil {
		return resumeextract.Professional{}, err
	}
	out := st.Professional()
	out.Experience = history
	return out, nil
}

// WorkHistory renders the owner's bank as résumé work-history entries — the single shape
// every reader of the bank's experience shares, whether or not it also carries contacts.
func (s *Store) WorkHistory(ctx context.Context, userID int64) ([]resumeextract.Experience, error) {
	employments, err := s.ListEmployments(ctx, userID)
	if err != nil {
		return nil, err
	}
	atoms, err := s.ListAtoms(ctx, userID)
	if err != nil {
		return nil, err
	}
	return experienceFromBank(employments, atoms), nil
}

// ProfessionalFrom builds the candidate profile from a bank and a structured résumé.
//
// The division is the point of the whole change. Work history comes from the bank, which
// accumulates — so evidence a candidate confirmed in a chat, and that appears in no
// uploaded file, is scored by the fit chain and seeded into their next CV. Education,
// languages, the summary and the years estimate keep coming from the structure, which owns
// them and has no accumulation problem.
//
// The structure's OWN experience is ignored rather than used as a fallback. A bank that
// failed to seed produces no work history, and the fit chain treats that as "no analysis"
// — which is the correct degradation: scoring a candidate against a work history nothing
// owns is worse than not scoring them.
//
// Contacts never cross over. The projection's field set stays the whitelist
// resumeextract.Professional already defines, so a field added there is withheld until it
// is added deliberately.
func ProfessionalFrom(st resumeextract.Structured, employments []Employment, atoms []Atom) resumeextract.Professional {
	out := st.Professional()
	out.Experience = experienceFromBank(employments, atoms)
	return out
}

// experienceFromBank renders the bank as work-history entries, in the order the store
// returned the employments (current roles first, most recent within that).
//
// Only publishable atoms become highlights: an agent's own inference is not something the
// candidate stands behind, and this projection feeds both the model that scores their fit
// and the CV seeded from it.
func experienceFromBank(employments []Employment, atoms []Atom) []resumeextract.Experience {
	highlights := make(map[uuid.UUID][]string, len(employments))
	var placeless []string
	for _, atom := range atoms {
		if !atom.Provenance.Publishable() {
			continue
		}
		if atom.EmploymentID == nil {
			placeless = append(placeless, atom.Claim)
			continue
		}
		highlights[*atom.EmploymentID] = append(highlights[*atom.EmploymentID], atom.Claim)
	}

	out := make([]resumeextract.Experience, 0, len(employments)+1)
	for _, e := range employments {
		// A role with no evidence under it is still career history: job titles matter to
		// the fit chain before their bullets do.
		out = append(out, resumeextract.Experience{
			Title:      e.Role,
			Company:    e.Company,
			Location:   e.Location,
			Start:      e.Start,
			End:        e.End,
			Summary:    e.Summary,
			Highlights: highlights[e.ID],
			Stack:      e.Stack,
		})
	}

	// Evidence with no place is still evidence — often the most valuable kind, since it is
	// what a candidate volunteered in conversation rather than wrote on a CV. It is carried
	// in an entry that names NO place rather than under an invented one: an empty company
	// is how this shape says "no place", and inventing a heading would be a fabrication in
	// the projection whose whole job is to carry only what is true.
	if len(placeless) > 0 {
		out = append(out, resumeextract.Experience{Highlights: placeless})
	}
	return out
}
