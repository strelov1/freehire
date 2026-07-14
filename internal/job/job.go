// Package job defines the Job domain aggregate: the core posting entity with a
// single guarded construction door. A Job can only be built through New (a fresh
// posting) or loaded through a Repository (a stored one); its state is unexported
// so no caller can assemble one bypassing the deterministic facet derivation.
// This makes "a Job always carries facets consistent with its source fields" a
// type-enforced invariant rather than a convention every write path must remember.
package job

import (
	"errors"
	"time"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/jobderive"
	"github.com/strelov1/freehire/internal/normalize"
	"github.com/strelov1/freehire/internal/pgconv"
)

// ErrInvalidDraft is returned by New when a draft lacks the identity a job
// requires: source, external id (together the dedup key), and a title.
var ErrInvalidDraft = errors.New("job: draft missing source, external id, or title")

// Draft is the source-agnostic input to New: the raw posting fields a write path
// supplies before derivation. The caller resolves Source/ExternalID (the dedup
// identity) so the job package stays free of the source registry. WorkMode,
// Seniority, Category, Skills, and ExperienceYearsMin are optional structured
// signals from the adapter that take precedence over the dictionaries.
type Draft struct {
	Source      string
	ExternalID  string
	URL         string
	Title       string
	Company     string
	Location    string
	Remote      bool
	Description string
	PostedAt    *time.Time

	WorkMode           string
	Seniority          string
	Category           string
	EmploymentType     string
	Skills             []string
	ExperienceYearsMin *int
}

// Fields is the readable projection of a Job: a plain DTO exposing every field for
// the wire projection and search indexing. A Fields can be freely constructed, but
// it is not a Job and cannot be persisted, so the construction invariant holds.
type Fields struct {
	Source      string
	ExternalID  string
	URL         string
	Title       string
	Company     string
	CompanySlug string
	PublicSlug  string
	Location    string
	Remote      bool
	Description string
	PostedAt    *time.Time

	// deterministic dictionary facets
	Countries []string
	Regions   []string
	Cities    []string
	WorkMode  string
	Skills    []string
	Seniority string
	Category  string
	// IsTech is the tri-state technical/non-technical signal (nil = unknown), derived
	// from title + category; served as a filterable facet. See jobderive.Derived.IsTech.
	IsTech *bool

	// synthetic enrichment facets (deterministic stand-ins)
	PostingLanguage    string
	EmploymentType     string
	EducationLevel     string
	EnglishLevel       string
	ExperienceYearsMin *int

	// lifecycle + enrichment provenance (0 = unenriched)
	ClosedAt          *time.Time
	EnrichmentVersion int32

	// Read-only projection fields, populated by a repository load and zero on a
	// fresh New. ID is the persistence id (the domain identity is Source+ExternalID);
	// ManuallyAdded is the moderator-authored provenance (created_by set). Enrichment
	// is the RAW decoded LLM payload before any dict fold — the projection folds the
	// dictionary columns over it. These never participate in the write surface.
	ID            int64
	ManuallyAdded bool
	Enrichment    enrich.Enrichment
	EnrichedAt    *time.Time
	CreatedAt     *time.Time
	UpdatedAt     *time.Time
}

// Job is the domain aggregate. Its state is unexported: the only ways to obtain a
// Job are New (fresh) and a Repository load (stored), so construction always runs
// through derivation. Read access for projection is via Fields.
type Job struct {
	f Fields
}

// New constructs a fresh Job from a draft, running the deterministic derivation
// (slugs + dictionary facets) internally. It is the single construction door for
// a not-yet-persisted posting. A fresh Job is open and unenriched.
func New(d Draft) (Job, error) {
	if d.Source == "" || d.ExternalID == "" || d.Title == "" {
		return Job{}, ErrInvalidDraft
	}
	// Strip any coordinate tail a source jammed into the free-text location before
	// it reaches both the facet derivation and the stored/displayed field — the
	// same order pipeline.normalizeJob uses.
	location := normalize.CleanLocation(d.Location)
	der := jobderive.Derive(jobderive.Input{
		Title:              d.Title,
		Company:            d.Company,
		Source:             d.Source,
		ExternalID:         d.ExternalID,
		Location:           location,
		Description:        d.Description,
		WorkMode:           d.WorkMode,
		Seniority:          d.Seniority,
		Category:           d.Category,
		EmploymentType:     d.EmploymentType,
		Skills:             d.Skills,
		ExperienceYearsMin: d.ExperienceYearsMin,
	})
	return Job{f: Fields{
		Source:      d.Source,
		ExternalID:  d.ExternalID,
		URL:         d.URL,
		Title:       d.Title,
		Company:     d.Company,
		CompanySlug: der.CompanySlug,
		PublicSlug:  der.PublicSlug,
		Location:    location,
		Remote:      d.Remote,
		Description: d.Description,
		PostedAt:    d.PostedAt,

		Countries: der.Countries,
		Regions:   der.Regions,
		Cities:    der.Cities,
		WorkMode:  der.WorkMode,
		Skills:    der.Skills,
		Seniority: der.Seniority,
		Category:  der.Category,
		IsTech:    der.IsTech,

		PostingLanguage:    der.PostingLanguage,
		EmploymentType:     der.EmploymentType,
		EducationLevel:     der.EducationLevel,
		EnglishLevel:       der.EnglishLevel,
		ExperienceYearsMin: der.ExperienceYearsMin,
	}}, nil
}

// Fields returns the readable projection of the aggregate. Slice and pointer
// fields alias the aggregate's; callers treat the result as read-only.
func (j Job) Fields() Fields { return j.f }

// UpsertParams maps the Fields-derived columns of a job to the generated UpsertJob
// params, so every write path (ingest, telegram extraction) shares one mapping
// instead of re-listing the columns. It covers only the fields carried on Fields;
// columns a caller derives separately (ContentHash, RoleFingerprint, or a PostedAt
// supplied outside the aggregate) are set on the returned struct after this call.
// Enrichment columns are deliberately excluded — SetJobEnrichment owns those.
func (f Fields) UpsertParams() db.UpsertJobParams {
	return db.UpsertJobParams{
		Source:      f.Source,
		ExternalID:  f.ExternalID,
		URL:         f.URL,
		Title:       f.Title,
		Company:     f.Company,
		CompanySlug: f.CompanySlug,
		PublicSlug:  f.PublicSlug,
		Location:    f.Location,
		Remote:      f.Remote,
		Description: f.Description,
		PostedAt:    pgconv.Timestamptz(f.PostedAt),
		Countries:   f.Countries,
		Regions:     f.Regions,
		Cities:      f.Cities,
		WorkMode:    f.WorkMode,
		Skills:      f.Skills,
		Seniority:   f.Seniority,
		Category:    f.Category,
		IsTech:      pgconv.Bool(f.IsTech),

		PostingLanguage:    f.PostingLanguage,
		EmploymentType:     f.EmploymentType,
		EducationLevel:     f.EducationLevel,
		EnglishLevel:       f.EnglishLevel,
		ExperienceYearsMin: pgconv.Int4(f.ExperienceYearsMin),
	}
}

// IsOpen reports whether the job is live (not soft-closed).
func (j Job) IsOpen() bool { return j.f.ClosedAt == nil }
