// Package userprofile is the single-per-user profile use case: a signed-in user records
// their professional self — a non-empty set of specializations (job categories) and a
// non-empty set of skills — and can fetch, save (create-or-replace), and clear that one
// profile. It owns validation (the specialization vocabulary and cap, skill
// normalization and cap); the Repository owns persistence and maps the no-row condition onto
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

	"github.com/strelov1/freehire/internal/dict/vocab"
)

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrInvalidSpecialization is a specialization outside the category vocabulary
	// (mapped to 400).
	ErrInvalidSpecialization = errors.New("userprofile: specialization is not a known category")
	// ErrEmptySpecializations is a profile whose specializations are empty after
	// normalization (mapped to 400).
	ErrEmptySpecializations = errors.New("userprofile: at least one specialization is required")
	// ErrTooManySpecializations is a specialization set past MaxSpecializations (mapped to 400).
	ErrTooManySpecializations = errors.New("userprofile: too many specializations")
	// ErrEmptySkills is a profile whose skills are empty after normalization (mapped to 400).
	ErrEmptySkills = errors.New("userprofile: at least one skill is required")
	// ErrTooManySkills is a wanted or avoided skill set past maxSkills (mapped to 400).
	ErrTooManySkills = errors.New("userprofile: too many skills")
	// ErrInvalidSeniority is a seniority outside the seniority vocabulary (mapped to 400).
	ErrInvalidSeniority = errors.New("userprofile: seniority is not a known level")
	// ErrNotFound is the caller having no profile yet (mapped to a null payload on GET,
	// 404 on the verdict/ATS sub-resources).
	ErrNotFound = errors.New("userprofile: not found")
	// ErrConflict is a guarded write (UpsertIfUnchanged) landing after the row it read
	// changed underneath it — used internally by MergeSkills' retry loop, never
	// returned to a Save() caller.
	ErrConflict = errors.New("userprofile: profile changed concurrently")
)

// mergeSkillsMaxAttempts bounds MergeSkills' read-merge-write retry when a concurrent
// Save() keeps winning the race. MergeSkills is a background courtesy sync (see its own
// doc comment) with no user waiting on it, so giving up quietly after a few attempts is
// the right failure mode, not an unbounded retry loop.
const mergeSkillsMaxAttempts = 3

// MaxSpecializations caps how many specializations one profile may combine; the
// migration's cardinality CHECK is the backstop. Exported so the HTTP layer names the
// bound in its 400 rather than repeating the number, and so the two cannot drift.
//
// Raised from 5 to 10 (migration 0135): a CV resolves several categories on its own and
// the profile form unions them into whatever is already picked, so a candidate whose work
// genuinely spans backend, data, cloud and a few more hit the bound without ever choosing
// to. 10 still bounds the set — it is a profile's role list, not a catalogue.
const MaxSpecializations = 10

// maxSkills caps how many skills one profile may list, wanted or avoided. The wanted set is
// not just stored: the coverage verdict turns it into one `skills != "<skill>"` AND group per
// element (see search.AndNotSkills), so an unbounded list would balloon that Meilisearch
// filter on every later read of the profile — against the same index that serves public
// search. The migration's cardinality CHECK is the backstop.
//
// The number is set by what real profiles hold, not by symmetry with the stateless coverage
// endpoint's 100. Measured on prod: of 131 profiles, 89% list 50 skills or fewer, but the
// largest lists 90 — so a ceiling of 100 left a live user 10 of headroom, and the CV-autofill
// path unions extracted skills into whatever is already in the form. 200 keeps that user
// clear of the bound while still bounding the filter: 90 groups already run against prod's
// index today, and the guard is against a list of 10^5, not against a long CV.
const maxSkills = 200

// maxSkillLen bounds one skill's length. The longest canonical form the skill dictionary can
// emit is 30 characters, so this leaves generous room for a free-text entry while keeping a
// single value from reaching the search filter as a multi-kilobyte literal.
const maxSkillLen = 64

// Profile is the user's saved professional profile: their specializations (job
// categories) and skills, plus optional location preferences kept as raw JSON
// (persisted and served verbatim). It is the package's domain type, decoupled from
// the generated db row — the internal bookkeeping columns (created_at/updated_at) are
// deliberately omitted so a schema change never ripples into handlers.
type Profile struct {
	UserID              int64
	Specializations     []string
	Skills              []string
	Seniorities         []string
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
	Upsert(ctx context.Context, userID int64, specializations, skills, seniorities, excludedSkills []string, locationPreferences json.RawMessage) (Profile, error)
	// UpsertIfUnchanged behaves like Upsert but writes only when the row's updated_at
	// still equals expectedUpdatedAt — the guard MergeSkills needs because its merge is
	// computed from a prior Get outside any transaction, so a Save() landing in that
	// gap must not be silently overwritten by a write built from a now-stale snapshot.
	// No matching row (updated_at moved, or the profile was deleted) maps to ErrConflict.
	UpsertIfUnchanged(ctx context.Context, userID int64, specializations, skills, seniorities, excludedSkills []string, locationPreferences json.RawMessage, expectedUpdatedAt time.Time) (Profile, error)
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
// normalized and must be non-empty; the seniorities are normalized (each a known level,
// deduped, may be empty); the excluded skills are normalized (deduped, may be empty) and
// any that also appear in skills are dropped — a skill cannot be both wanted and avoided,
// and the wanted set wins; the optional location block is validated and normalized (or
// stored NULL when nil/empty). It is a create-or-replace: the first save inserts, later
// saves overwrite.
func (s *Service) Save(ctx context.Context, userID int64, specializations, skills, seniorities, excludedSkills []string, loc *LocationPreferences) (Profile, error) {
	specs, err := normalizeSpecializations(specializations)
	if err != nil {
		return Profile{}, err
	}
	normalized, err := normalizeSkills(skills)
	if err != nil {
		return Profile{}, err
	}
	levels, err := normalizeSeniorities(seniorities)
	if err != nil {
		return Profile{}, err
	}
	avoided, err := normalizeSkillList(excludedSkills)
	if err != nil {
		return Profile{}, err
	}
	excluded := subtractSkills(avoided, normalized)
	locJSON, err := normalizeLocationPreferences(loc)
	if err != nil {
		return Profile{}, err
	}
	return s.repo.Upsert(ctx, userID, specs, normalized, levels, excluded, locJSON)
}

// Delete removes the user's profile. It is idempotent — deleting when none exists is not
// an error.
func (s *Service) Delete(ctx context.Context, userID int64) error {
	return s.repo.Delete(ctx, userID)
}

// MergeSkills folds skills the caller did not directly ask to save — freshly banked
// experience-bank evidence — into an existing profile, so the profile stays current with
// the bank without the candidate re-declaring what it already proves. It is a courtesy
// update, not profile management: a user with no saved profile gets no side effect (the
// same "do not invent one" rule the profile's own Save enforces), and it never removes a
// skill, overwrites specializations, excluded_skills or location preferences, or errors
// past the skill cap — it silently adds only as many of the new skills as still fit,
// mirroring how a manual claim behaves when the profile is near the limit.
//
// The merge (which skills still fit, what the rest of the profile currently holds) is
// computed in Go from a Get read outside any transaction, so a concurrent Save() landing
// in that gap must not be silently reverted by a write built from that now-stale
// snapshot. The write is guarded on the row's updated_at (UpsertIfUnchanged); a
// concurrent write in the gap is retried from a fresh read up to mergeSkillsMaxAttempts
// times before giving up quietly — the same courtesy-update posture as the rest of this
// method, just extended to the race as well as to the skill cap.
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

// mergeSkillsOnce is one read-merge-write attempt of MergeSkills. It returns ErrConflict
// when the guarded write lost a race to a concurrent Save(), for the caller to retry.
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
		// No version to guard the write on — write unconditionally, same as before
		// this fix. A profile read from the real Repository always carries a non-nil
		// UpdatedAt (updated_at is NOT NULL); this only happens against a Repository
		// fake that leaves it unset.
		_, err = s.repo.Upsert(ctx, userID, profile.Specializations, merged, profile.Seniorities, profile.ExcludedSkills, profile.LocationPreferences)
		return err
	}
	_, err = s.repo.UpsertIfUnchanged(ctx, userID, profile.Specializations, merged, profile.Seniorities, profile.ExcludedSkills, profile.LocationPreferences, *profile.UpdatedAt)
	return err
}

// mergeSkills appends the new skills that are not already held and not excluded, capped at
// maxSkills, preserving the existing order and existing entries untouched. It reports
// whether anything was actually appended, so a caller with nothing new to add can skip a
// write entirely.
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

// normalizeSpecializations trims each value, drops blanks, deduplicates (preserving
// first-seen order), and checks membership in the controlled category vocabulary (the same
// enum the rest of the app validates against). It returns ErrEmptySpecializations if nothing
// remains, ErrInvalidSpecialization for an unknown category, and ErrTooManySpecializations
// past MaxSpecializations — mirroring normalizeSkills.
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
		if !slices.Contains(vocab.CategoryValues, spec) {
			return nil, ErrInvalidSpecialization
		}
		seen[spec] = struct{}{}
		out = append(out, spec)
	}
	if len(out) == 0 {
		return nil, ErrEmptySpecializations
	}
	if len(out) > MaxSpecializations {
		return nil, ErrTooManySpecializations
	}
	return out, nil
}

// normalizeSeniorities trims each value, drops blanks, deduplicates (preserving
// first-seen order), and checks membership in the controlled seniority vocabulary.
// Unlike normalizeSpecializations, an empty result is valid — a profile need not state a
// level — and there is no cap: the vocabulary itself only has 8 values.
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

// normalizeSkills normalizes the wanted skills and additionally requires the set to be
// non-empty — a profile without skills has no meaning.
func normalizeSkills(skills []string) ([]string, error) {
	out, err := normalizeSkillList(skills)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, ErrEmptySkills
	}
	return out, nil
}

// normalizeSkillList lowercases, trims, and deduplicates a skill list (preserving first-seen
// order), and caps it at maxSkills. A per-value problem drops that value: blanks, and anything
// past maxSkillLen — a string that long is not a skill, and losing one junk entry beats
// failing an otherwise valid save. A whole-list problem errors: a set past the cardinality cap
// returns ErrTooManySkills, mirroring normalizeSpecializations. An empty result is valid here
// (the user need not avoid anything), and the slice is always non-nil so the value persists as
// an empty array, not NULL.
func normalizeSkillList(skills []string) ([]string, error) {
	out := make([]string, 0, len(skills))
	seen := make(map[string]struct{}, len(skills))
	for _, raw := range skills {
		skill := strings.ToLower(strings.TrimSpace(raw))
		if skill == "" || len(skill) > maxSkillLen {
			continue
		}
		if _, dup := seen[skill]; dup {
			continue
		}
		seen[skill] = struct{}{}
		out = append(out, skill)
	}
	if len(out) > maxSkills {
		return nil, ErrTooManySkills
	}
	return out, nil
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
