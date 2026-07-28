package experience

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/skilltag"
)

// ImportEntry is one role as a source states it: the place, and the achievements printed
// under it. It is deliberately not resumeextract's shape — the bank should not be coupled
// to one importer, and the mapping from a parsed CV lives at that seam instead.
type ImportEntry struct {
	Employment Employment
	Claims     []string
}

// ImportResult reports what an import did, for the caller to log. Nothing here counts a
// removal, because import performs none.
type ImportResult struct {
	EmploymentsCreated int
	EmploymentsFilled  int
	AtomsCreated       int
	AtomsSkipped       int // already banked under some spelling
}

// Import reconciles a source's work history into the bank. It is additive by
// construction: an employment already present has only its blanks filled, a claim already
// banked is counted and skipped, and nothing is ever deleted. A user who uploads a
// trimmed one-page CV must not lose the bank they built.
//
// An entry naming neither a company nor a role names no PLACE — but its claims are still
// achievements, so they are banked standing alone rather than discarded. The bias of this
// domain is that evidence is never lost because its context was unparseable.
//
// sourceRef records where the import came from (an upload stamp), so an atom can always
// be traced back to the artifact that produced it.
func (s *Store) Import(ctx context.Context, userID int64, entries []ImportEntry, sourceRef string) (ImportResult, error) {
	var result ImportResult

	for _, entry := range entries {
		employmentID, err := s.reconcileEmployment(ctx, userID, entry.Employment, &result)
		if err != nil {
			return result, err
		}
		for _, claim := range entry.Claims {
			atom := Atom{
				EmploymentID: employmentID,
				Claim:        claim,
				// Skills are mined from the bullet's OWN text. That text is prose, so
				// Parse — with its corroboration rule — is the right entry to the
				// dictionary here, unlike an explicitly-asserted skill list. The role's
				// stack stays on the employment: copying it onto every bullet would tag
				// one about hiring with the role's message broker.
				Skills:     skilltag.Parse(claim, skilltag.WithResumeAcronyms()),
				Provenance: ProvenanceCVImport,
				SourceRef:  sourceRef,
			}
			switch _, err := s.AddAtom(ctx, userID, atom); {
			case err == nil:
				result.AtomsCreated++
			case errors.Is(err, ErrAlreadyBanked):
				result.AtomsSkipped++
			case errors.Is(err, ErrEmptyClaim):
				// A blank bullet is noise in the parse, not a reason to fail the upload.
			default:
				return result, err
			}
		}
	}
	return result, nil
}

// reconcileEmployment returns the id of the place this entry belongs to, creating it or
// filling its blanks as needed. A nil id means the entry named no place, and its claims
// stand alone.
func (s *Store) reconcileEmployment(ctx context.Context, userID int64, e Employment, result *ImportResult) (*uuid.UUID, error) {
	e.Sanitize()
	if e.Company == "" && e.Role == "" {
		return nil, nil
	}
	if e.Kind == "" {
		e.Kind = KindJob
	}

	existing, err := s.FindEmployment(ctx, userID, e.Company, e.Role)
	switch {
	case err == nil:
		filled, err := s.FillEmploymentBlanks(ctx, existing.ID, userID, e)
		if err != nil {
			return nil, err
		}
		result.EmploymentsFilled++
		return &filled.ID, nil
	case errors.Is(err, ErrNotFound):
		created, err := s.CreateEmployment(ctx, userID, e)
		if err != nil {
			return nil, err
		}
		result.EmploymentsCreated++
		return &created.ID, nil
	default:
		return nil, err
	}
}
