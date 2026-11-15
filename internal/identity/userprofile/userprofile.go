// Package userprofile is the one-per-user profile use case: fetch, save (create-or-replace)
// and clear a user's specializations, skills and preferences, which may be empty while being
// filled in. It owns validation; the Repository owns persistence and maps no-row to
// ErrNotFound. How a profile is consumed (scoring, feeds, notifications) lives elsewhere.
package userprofile

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrInvalidSpecialization is a specialization outside the category vocabulary
	// (mapped to 400).
	ErrInvalidSpecialization = errors.New("userprofile: specialization is not a known category")
	// ErrTooManySpecializations is a specialization set past MaxSpecializations (mapped to 400).
	ErrTooManySpecializations = errors.New("userprofile: too many specializations")
	// ErrTooManySkills is a wanted or avoided skill set past maxSkills (mapped to 400).
	ErrTooManySkills = errors.New("userprofile: too many skills")
	// ErrTooManySources is an excluded-sources set past maxExcludedCount (mapped to 400).
	ErrTooManySources = errors.New("userprofile: too many excluded sources")
	// ErrTooManyCompanies is an excluded-companies set past maxExcludedCount (mapped to 400).
	ErrTooManyCompanies = errors.New("userprofile: too many excluded companies")
	// ErrInvalidSeniority is a seniority outside the seniority vocabulary (mapped to 400).
	ErrInvalidSeniority = errors.New("userprofile: seniority is not a known level")
	// ErrNotFound is the caller having no profile yet (mapped to a null payload on GET,
	// 404 on the verdict/ATS sub-resources).
	ErrNotFound = errors.New("userprofile: not found")
	// ErrConflict is a guarded write losing a race: either the read row changed or
	// a missing profile was created. Read-modify-write callers can reload and retry.
	// A full Save is an unconditional replacement and does not return this error.
	ErrConflict = errors.New("userprofile: profile changed concurrently")
)

// mergeSkillsMaxAttempts bounds MergeSkills' retry when a concurrent Save() keeps winning.
// It is a background courtesy sync, so giving up quietly is fine.
const mergeSkillsMaxAttempts = 3

// MaxSpecializations caps how many specializations one profile may combine; the migration's
// cardinality CHECK is the backstop. Exported so the HTTP layer names the bound in its 400.
// A CV resolves several categories and the form unions them in, so the cap must leave room.
const MaxSpecializations = 10

// maxSkills caps how many skills one profile may list, wanted or avoided. The coverage
// verdict turns the wanted set into one `skills != "<skill>"` AND group per element (see
// search.AndNotSkills), so an unbounded list would balloon a Meilisearch filter on the index
// serving public search. The migration's cardinality CHECK is the backstop.
//
// Sized above real profiles plus CV-autofill unions; it guards against absurd lists, not
// long CVs.
const maxSkills = 200

// maxSkillLen bounds one skill's length: the longest canonical form is 30 characters, and
// this keeps a multi-kilobyte literal out of the search filter.
const maxSkillLen = 64

// maxExcludedCount caps excluded_sources and excluded_companies, each independently. Equal
// to maxSkills for consistency; neither set is expanded into a search filter today.
const maxExcludedCount = maxSkills

// maxExcludedLen bounds one excluded_sources or excluded_companies value's length; company
// slugs run longer than adapter names, so one generous bound serves both.
const maxExcludedLen = 100

// Profile is the user's saved professional profile; location preferences are raw JSON,
// persisted and served verbatim. It is decoupled from the generated db row so a schema
// change does not ripple into handlers.
type Profile struct {
	UserID              int64
	Specializations     []string
	Skills              []string
	Seniorities         []string
	ExcludedSkills      []string
	ExcludedSources     []string
	JobLanguages        []string
	ExcludedCompanies   []string
	LocationPreferences json.RawMessage
	CreatedAt           *time.Time
	UpdatedAt           *time.Time
}

// Repository is the persistence contract for the user profile, scoped by user_id. Get maps
// a missing row to ErrNotFound; Delete is idempotent.
type Repository interface {
	Get(ctx context.Context, userID int64) (Profile, error)
	Upsert(ctx context.Context, userID int64, specializations, skills, seniorities, excludedSkills, excludedSources, excludedCompanies, jobLanguages []string, locationPreferences json.RawMessage) (Profile, error)
	// CreateIfAbsent inserts only when no profile exists, or returns ErrConflict.
	CreateIfAbsent(ctx context.Context, userID int64, specializations, skills, seniorities, excludedSkills, excludedSources, excludedCompanies, jobLanguages []string, locationPreferences json.RawMessage) (Profile, error)
	// UpsertIfUnchanged is Upsert guarded on updated_at still equalling expectedUpdatedAt, so a
	// write built from a stale snapshot cannot overwrite a concurrent Save. No matching row
	// (updated_at moved, or profile deleted) maps to ErrConflict.
	UpsertIfUnchanged(ctx context.Context, userID int64, specializations, skills, seniorities, excludedSkills, excludedSources, excludedCompanies, jobLanguages []string, locationPreferences json.RawMessage, expectedUpdatedAt time.Time) (Profile, error)
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

// Save validates and upserts the user's profile: the first save inserts, later saves
// overwrite. Every list is normalized and may be empty. Excluded skills that also appear in
// skills are dropped (the wanted set wins). The location block is stored NULL when nil or empty.
func (s *Service) Save(ctx context.Context, userID int64, specializations, skills, seniorities, excludedSkills, excludedSources, excludedCompanies []string, loc *LocationPreferences, jobLanguages ...string) (Profile, error) {
	n, err := normalizeProfile(specializations, skills, seniorities, excludedSkills, excludedSources, excludedCompanies, loc, jobLanguages)
	if err != nil {
		return Profile{}, err
	}
	return s.repo.Upsert(ctx, userID, n.specs, n.skills, n.levels, n.excluded, n.sources, n.companies, n.languages, n.loc)
}

// CreateIfAbsent normalizes a first partial update and inserts it only if the profile
// is still absent. ErrConflict asks a read-modify-write caller to reload and merge.
func (s *Service) CreateIfAbsent(ctx context.Context, userID int64, specializations, skills, seniorities, excludedSkills, excludedSources, excludedCompanies []string, loc *LocationPreferences, jobLanguages ...string) (Profile, error) {
	n, err := normalizeProfile(specializations, skills, seniorities, excludedSkills, excludedSources, excludedCompanies, loc, jobLanguages)
	if err != nil {
		return Profile{}, err
	}
	return s.repo.CreateIfAbsent(ctx, userID, n.specs, n.skills, n.levels, n.excluded, n.sources, n.companies, n.languages, n.loc)
}

// SaveIfUnchanged is Save for a caller that computed the profile from a prior Get: it writes
// only when updated_at still equals expected, else ErrConflict so the caller can re-read.
// Use CreateIfAbsent for a missing-row snapshot.
func (s *Service) SaveIfUnchanged(ctx context.Context, userID int64, expected time.Time, specializations, skills, seniorities, excludedSkills, excludedSources, excludedCompanies []string, loc *LocationPreferences, jobLanguages ...string) (Profile, error) {
	n, err := normalizeProfile(specializations, skills, seniorities, excludedSkills, excludedSources, excludedCompanies, loc, jobLanguages)
	if err != nil {
		return Profile{}, err
	}
	return s.repo.UpsertIfUnchanged(ctx, userID, n.specs, n.skills, n.levels, n.excluded, n.sources, n.companies, n.languages, n.loc, expected)
}

// normalizedProfile is a profile after validation, ready for the repository.
type normalizedProfile struct {
	specs, skills, levels, excluded, sources, companies, languages []string
	loc                                                            json.RawMessage
}

func normalizeProfile(specializations, skills, seniorities, excludedSkills, excludedSources, excludedCompanies []string, loc *LocationPreferences, jobLanguages []string) (normalizedProfile, error) {
	var n normalizedProfile
	var err error
	if n.specs, err = normalizeSpecializations(specializations); err != nil {
		return n, err
	}
	if n.skills, err = normalizeSkillList(skills); err != nil {
		return n, err
	}
	if n.levels, err = normalizeSeniorities(seniorities); err != nil {
		return n, err
	}
	avoided, err := normalizeSkillList(excludedSkills)
	if err != nil {
		return n, err
	}
	n.excluded = subtractSkills(avoided, n.skills)
	if n.sources, err = normalizeExcludedSet(excludedSources, maxExcludedLen, maxExcludedCount, ErrTooManySources); err != nil {
		return n, err
	}
	if n.companies, err = normalizeExcludedSet(excludedCompanies, maxExcludedLen, maxExcludedCount, ErrTooManyCompanies); err != nil {
		return n, err
	}
	if n.languages, err = normalizeJobLanguages(jobLanguages); err != nil {
		return n, err
	}
	n.loc, err = normalizeLocationPreferences(loc)
	return n, err
}

// Delete removes the user's profile; deleting a missing one is not an error.
func (s *Service) Delete(ctx context.Context, userID int64) error {
	return s.repo.Delete(ctx, userID)
}

// MergeSkills folds skills the caller did not directly save (freshly banked experience
// evidence) into an existing profile. It is a courtesy update: a user with no profile gets
// none created, nothing else is overwritten or removed, and it adds only as many skills as
// still fit under the cap instead of erroring.
//
// The merge is computed from a Get outside any transaction, so the write is guarded on
// updated_at (UpsertIfUnchanged) and retried from a fresh read up to mergeSkillsMaxAttempts
// times before giving up quietly.
func (s *Service) MergeSkills(ctx context.Context, userID int64, skills []string) error {
	var err error
	for attempt := 0; attempt < mergeSkillsMaxAttempts; attempt++ {
		err = s.mergeSkillsOnce(ctx, userID, skills)
		if !errors.Is(err, ErrConflict) {
			return err
		}
	}
	return err
}

// mergeSkillsOnce is one read-merge-write attempt; ErrConflict means it lost a race to Save().
func (s *Service) mergeSkillsOnce(ctx context.Context, userID int64, skills []string) error {
	profile, err := s.repo.Get(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	merged, changed := mergeSkills(profile.Skills, skills, profile.ExcludedSkills)
	if !changed {
		return nil
	}
	if profile.UpdatedAt == nil {
		// No version to guard on, so write unconditionally. The real Repository always sets
		// UpdatedAt (NOT NULL); only fakes leave it nil.
		_, err = s.repo.Upsert(ctx, userID, profile.Specializations, merged, profile.Seniorities, profile.ExcludedSkills, profile.ExcludedSources, profile.ExcludedCompanies, profile.JobLanguages, profile.LocationPreferences)
		return err
	}
	_, err = s.repo.UpsertIfUnchanged(ctx, userID, profile.Specializations, merged, profile.Seniorities, profile.ExcludedSkills, profile.ExcludedSources, profile.ExcludedCompanies, profile.JobLanguages, profile.LocationPreferences, *profile.UpdatedAt)
	return err
}

// mergeSkills appends additions that are neither held nor excluded, up to maxSkills,
// leaving existing entries untouched. It reports whether anything was appended so the
// caller can skip the write.
func mergeSkills(held, additions, excluded []string) ([]string, bool) {
	have := make(map[string]struct{}, len(held))
	for _, s := range held {
		have[strings.ToLower(s)] = struct{}{}
	}
	avoid := make(map[string]struct{}, len(excluded))
	for _, s := range excluded {
		avoid[strings.ToLower(s)] = struct{}{}
	}

	merged := held
	changed := false
	for _, raw := range additions {
		if len(merged) >= maxSkills {
			break
		}
		skill := strings.ToLower(strings.TrimSpace(raw))
		if skill == "" || len(skill) > maxSkillLen {
			continue
		}
		if _, dup := have[skill]; dup {
			continue
		}
		if _, isExcluded := avoid[skill]; isExcluded {
			continue
		}
		have[skill] = struct{}{}
		merged = append(merged, skill)
		changed = true
	}
	return merged, changed
}

// normalizeSpecializations trims, drops blanks, dedupes (first-seen order) and checks
// membership in the category vocabulary. Empty is valid. Errors: ErrInvalidSpecialization,
// ErrTooManySpecializations.
//
// The retired vocab.RetiredCategoryManagement is dropped like a blank, not refused: the form
// resends the stored list on every save, so refusing it would block saving for profiles
// still carrying a value the picker no longer shows.
func normalizeSpecializations(specializations []string) ([]string, error) {
	out := make([]string, 0, len(specializations))
	seen := make(map[string]struct{}, len(specializations))
	for _, raw := range specializations {
		spec := strings.TrimSpace(raw)
		if spec == "" || spec == vocab.RetiredCategoryManagement {
			continue
		}
		if _, dup := seen[spec]; dup {
			continue
		}
		if !slices.Contains(vocab.CategoryValues, spec) {
			return nil, ErrInvalidSpecialization
		}
		seen[spec] = struct{}{}
		out = append(out, spec)
	}
	if len(out) > MaxSpecializations {
		return nil, ErrTooManySpecializations
	}
	return out, nil
}

// normalizeSeniorities trims, drops blanks, dedupes (first-seen order) and checks membership
// in the seniority vocabulary. Empty is valid; no cap, the vocabulary is small.
func normalizeSeniorities(seniorities []string) ([]string, error) {
	out := make([]string, 0, len(seniorities))
	seen := make(map[string]struct{}, len(seniorities))
	for _, raw := range seniorities {
		level := strings.TrimSpace(raw)
		if level == "" {
			continue
		}
		if _, dup := seen[level]; dup {
			continue
		}
		if !slices.Contains(vocab.SeniorityValues, level) {
			return nil, ErrInvalidSeniority
		}
		seen[level] = struct{}{}
		out = append(out, level)
	}
	return out, nil
}

// normalizeSkillList is normalizeExcludedSet with the skill bounds and ErrTooManySkills.
func normalizeSkillList(skills []string) ([]string, error) {
	return normalizeExcludedSet(skills, maxSkillLen, maxSkills, ErrTooManySkills)
}

// normalizeExcludedSet lowercases, trims and dedupes a list (first-seen order). A value past
// maxLen is dropped (junk beats failing the save); a set past maxCount fails with errTooMany.
// The result is always non-nil so it persists as an empty array, not NULL.
func normalizeExcludedSet(values []string, maxLen, maxCount int, errTooMany error) ([]string, error) {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := strings.ToLower(strings.TrimSpace(raw))
		if value == "" || len(value) > maxLen {
			continue
		}
		if _, dup := seen[value]; dup {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) > maxCount {
		return nil, errTooMany
	}
	return out, nil
}

// subtractSkills returns the values in a not in remove, keeping a's order. It keeps wanted
// and excluded skills disjoint so a filter never emits skills = X AND skills != X.
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
