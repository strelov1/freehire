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
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

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

// Profile is the user's saved professional profile: their specializations (job
// categories) and skills, plus optional location preferences kept as raw JSON
// (persisted and served verbatim). It is the package's domain type, decoupled from
// the generated db row — the internal bookkeeping columns (created_at/updated_at) are
// deliberately omitted so a schema change never ripples into handlers.
type Profile struct {
	UserID              int64
	Specializations     []string
	Skills              []string
	ExcludedSkills      []string
	LocationPreferences json.RawMessage
	CreatedAt           *time.Time
	UpdatedAt           *time.Time
}

// Repository is the persistence contract for the single user profile. Every method is
// user-scoped by user_id. Get maps a missing row to ErrNotFound; Upsert creates or
// replaces; Delete is idempotent (no row is not an error). Implementations map the
// generated db row to Profile, so the use case never sees db.*.
type Repository interface {
	Get(ctx context.Context, userID int64) (Profile, error)
	Upsert(ctx context.Context, userID int64, specializations, skills, excludedSkills []string, locationPreferences json.RawMessage) (Profile, error)
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
func (s *Service) Get(ctx context.Context, userID int64) (Profile, error) {
	return s.repo.Get(ctx, userID)
}

// Save validates and upserts the user's single profile. The specializations are
// normalized (each a known category, deduped, non-empty, capped); the skills are
// normalized and must be non-empty; the excluded skills are normalized (deduped, may be
// empty) and any that also appear in skills are dropped — a skill cannot be both wanted
// and avoided, and the wanted set wins; the optional location block is validated and
// normalized (or stored NULL when nil/empty). It is a create-or-replace: the first save
// inserts, later saves overwrite.
func (s *Service) Save(ctx context.Context, userID int64, specializations, skills, excludedSkills []string, loc *LocationPreferences) (Profile, error) {
	specs, err := normalizeSpecializations(specializations)
	if err != nil {
		return Profile{}, err
	}
	normalized, err := normalizeSkills(skills)
	if err != nil {
		return Profile{}, err
	}
	excluded := subtractSkills(normalizeExcludedSkills(excludedSkills), normalized)
	locJSON, err := normalizeLocationPreferences(loc)
	if err != nil {
		return Profile{}, err
	}
	return s.repo.Upsert(ctx, userID, specs, normalized, excluded, locJSON)
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
	out := normalizeExcludedSkills(skills)
	if len(out) == 0 {
		return nil, ErrEmptySkills
	}
	return out, nil
}

// normalizeExcludedSkills lowercases, trims, and deduplicates a skill list (preserving
// first-seen order), dropping blanks. Unlike normalizeSkills it never errors: an empty set
// is valid (the user need not avoid anything). It always returns a non-nil slice so the
// value persists as an empty array, not NULL.
func normalizeExcludedSkills(skills []string) []string {
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
	return out
}

// subtractSkills returns the values in a that are not in remove, preserving a's order. It
// enforces the invariant that the wanted and excluded skill sets never overlap: a skill
// listed as both wanted and avoided is dropped from the excluded set (the wanted set
// wins), so a committed filter never emits contradictory skills = X AND skills != X.
func subtractSkills(a, remove []string) []string {
	if len(remove) == 0 {
		return a
	}
	drop := make(map[string]struct{}, len(remove))
	for _, s := range remove {
		drop[s] = struct{}{}
	}
	out := make([]string, 0, len(a))
	for _, s := range a {
		if _, skip := drop[s]; skip {
			continue
		}
		out = append(out, s)
	}
	return out
}
