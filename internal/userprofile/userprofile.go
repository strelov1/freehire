// Package userprofile is the single-per-user profile use case: a signed-in user records
// their professional self — a non-empty set of specializations (job categories) and a
// non-empty set of skills — and can fetch, save (create-or-replace), and clear that one
// profile. It owns validation (the specialization vocabulary and cap, skill
// normalization); the Repository owns persistence and maps the no-row condition onto
// ErrNotFound. There is at most one profile per user (keyed by user_id), so there is no
// name, no id, and no cap. How a profile is consumed (match scoring, ranked feeds,
// notifications) lives outside this package.
package userprofile

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
)

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrInvalidSpecialization is a specialization outside the category vocabulary
	// (mapped to 400).
	ErrInvalidSpecialization = errors.New("userprofile: specialization is not a known category")
	// ErrEmptySpecializations is a profile whose specializations are empty after
	// normalization (mapped to 400).
	ErrEmptySpecializations = errors.New("userprofile: at least one specialization is required")
	// ErrTooManySpecializations is a specialization set past maxSpecializations (mapped to 400).
	ErrTooManySpecializations = errors.New("userprofile: too many specializations")
	// ErrEmptySkills is a profile whose skills are empty after normalization (mapped to 400).
	ErrEmptySkills = errors.New("userprofile: at least one skill is required")
	// ErrNotFound is the caller having no profile yet (mapped to a null payload on GET,
	// 404 on the verdict/ATS sub-resources).
	ErrNotFound = errors.New("userprofile: not found")
)

// maxSpecializations caps how many specializations one profile may combine; the
// migration's cardinality CHECK is the backstop.
const maxSpecializations = 5

// Repository is the persistence contract for the single user profile. Every method is
// user-scoped by user_id. Get maps a missing row to ErrNotFound; Upsert creates or
// replaces; Delete is idempotent (no row is not an error).
type Repository interface {
	Get(ctx context.Context, userID int64) (db.UserProfile, error)
	Upsert(ctx context.Context, p db.UpsertUserProfileParams) (db.UserProfile, error)
	Delete(ctx context.Context, userID int64) error
}

// Service implements the user-profile use cases.
type Service struct {
	repo Repository
}

// New creates a Service backed by the given Repository.
func New(repo Repository) *Service {
	return &Service{repo: repo}
}

// Get returns the user's profile, or ErrNotFound when they have not saved one yet.
func (s *Service) Get(ctx context.Context, userID int64) (db.UserProfile, error) {
	return s.repo.Get(ctx, userID)
}

// Save validates and upserts the user's single profile. The specializations are
// normalized (each a known category, deduped, non-empty, capped); the skills are
// normalized and must be non-empty. It is a create-or-replace: the first save inserts,
// later saves overwrite.
func (s *Service) Save(ctx context.Context, userID int64, specializations, skills []string) (db.UserProfile, error) {
	specs, err := normalizeSpecializations(specializations)
	if err != nil {
		return db.UserProfile{}, err
	}
	normalized, err := normalizeSkills(skills)
	if err != nil {
		return db.UserProfile{}, err
	}
	return s.repo.Upsert(ctx, db.UpsertUserProfileParams{
		UserID:          userID,
		Specializations: specs,
		Skills:          normalized,
	})
}

// Delete removes the user's profile. It is idempotent — deleting when none exists is not
// an error.
func (s *Service) Delete(ctx context.Context, userID int64) error {
	return s.repo.Delete(ctx, userID)
}

// normalizeSpecializations trims each value, drops blanks, deduplicates (preserving
// first-seen order), and checks membership in the controlled category vocabulary (the same
// enum the rest of the app validates against). It returns ErrEmptySpecializations if nothing
// remains, ErrInvalidSpecialization for an unknown category, and ErrTooManySpecializations
// past maxSpecializations — mirroring normalizeSkills.
func normalizeSpecializations(specializations []string) ([]string, error) {
	out := make([]string, 0, len(specializations))
	seen := make(map[string]struct{}, len(specializations))
	for _, raw := range specializations {
		spec := strings.TrimSpace(raw)
		if spec == "" {
			continue
		}
		if _, dup := seen[spec]; dup {
			continue
		}
		if !slices.Contains(enrich.CategoryValues, spec) {
			return nil, ErrInvalidSpecialization
		}
		seen[spec] = struct{}{}
		out = append(out, spec)
	}
	if len(out) == 0 {
		return nil, ErrEmptySpecializations
	}
	if len(out) > maxSpecializations {
		return nil, ErrTooManySpecializations
	}
	return out, nil
}

// normalizeSkills lowercases, trims, and deduplicates the skills (preserving first-seen
// order), dropping blanks. It returns ErrEmptySkills if nothing remains — a profile without
// skills has no meaning.
func normalizeSkills(skills []string) ([]string, error) {
	out := make([]string, 0, len(skills))
	seen := make(map[string]struct{}, len(skills))
	for _, raw := range skills {
		skill := strings.ToLower(strings.TrimSpace(raw))
		if skill == "" {
			continue
		}
		if _, dup := seen[skill]; dup {
			continue
		}
		seen[skill] = struct{}{}
		out = append(out, skill)
	}
	if len(out) == 0 {
		return nil, ErrEmptySkills
	}
	return out, nil
}
