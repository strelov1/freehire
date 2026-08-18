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

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/jobderive"
	"github.com/strelov1/freehire/internal/jobhash"
	"github.com/strelov1/freehire/internal/normalize"
	"github.com/strelov1/freehire/internal/pgconv"
)

// ErrInvalidDraft is returned by New when a draft lacks the identity a job
// requires: source, external id (together the dedup key), and a title.
var ErrInvalidDraft = errors.New("job: draft missing source, external id, or title")

// Salary is a stated compensation range in a currency's units over a period. A nil
// Min/Max means that bound was not stated. It carries the authoritative manual salary a
// submitter or moderator entered by hand — distinct from the LLM-enriched salary in the
// enrichment payload, which it takes precedence over in the effective projection.
type Salary struct {
	Min      *int
	Max      *int
	Currency string
	Period   string
}

// Draft is the source-agnostic input to New: the raw posting fields a write path
// supplies before derivation. The embedded jobderive.Input carries every field the
// deterministic derivation reads (identity, content, and the optional structured
// signals — see its docs); Draft adds only what derivation never touches: the URL,
// the remote flag, the source posted time, and the authoritative manual salary
// (nil when none stated). The caller resolves Source/ExternalID (the dedup
// identity) so the job package stays free of the source registry.
type Draft struct {
	jobderive.Input

	URL      string
	Remote   bool
	PostedAt *time.Time

	ManualSalary *Salary

	// CanonicalCompanySlug overrides the derived company slug with the one a merge elected
	// for this employer (company_slug_aliases). Empty for almost every posting, and empty
	// is the honest default: only the ingest pipeline can fill it, because only it has the
	// registry, and jobderive must stay a pure function of its input.
	//
	// It rides on the Draft rather than being applied to a built Job so the aggregate is
	// still constructed once and never mutated — a setter would let any caller re-key a
	// company, which is a decision the merge worker owns.
	CanonicalCompanySlug string
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

	// ManualSalary is the authoritative manual salary (nil when none). It is a base
	// field, never derived, and takes precedence over the enriched salary.
	ManualSalary *Salary
	// SourceSalary is the ATS's own structured salary (nil when the source stated
	// none), derived from jobderive's passthrough — see its Input docs. Wins over the
	// enrichment pass's LLM-derived guess but never over ManualSalary.
	SourceSalary *Salary

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
	// LastSeenAt is when a re-crawl last confirmed this posting still live (see
	// docs/agents/job-lifecycle.md) — the freshest "still open" evidence available
	// for an open job, used to estimate a rolling JobPosting.validThrough.
	LastSeenAt *time.Time
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
	in := d.Input
	in.Location = normalize.CleanLocation(in.Location)
	der := jobderive.Derive(in)
	if d.CanonicalCompanySlug != "" {
		der.CompanySlug = d.CanonicalCompanySlug
	}
	return Job{f: Fields{
		Source:      d.Source,
		ExternalID:  d.ExternalID,
		URL:         d.URL,
		Title:       d.Title,
		Company:     d.Company,
		CompanySlug: der.CompanySlug,
		PublicSlug:  der.PublicSlug,
		Location:    in.Location,
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

		// Authoritative manual salary is carried verbatim — never derived.
		ManualSalary: d.ManualSalary,
		SourceSalary: salaryFromParts(der.SalaryMin, der.SalaryMax, der.SalaryCurrency, der.SalaryPeriod),
	}}, nil
}

// salaryFromParts packages a structured salary's parts into a *Salary, or nil when
// neither bound is present — the shape SourceSalary and manualSalaryFromRow share.
func salaryFromParts(min, max *int, currency, period string) *Salary {
	if min == nil && max == nil {
		return nil
	}
	return &Salary{Min: min, Max: max, Currency: currency, Period: period}
}

// Fields returns the readable projection of the aggregate. Slice and pointer
// fields alias the aggregate's; callers treat the result as read-only.
func (j Job) Fields() Fields { return j.f }

// UpsertParams maps a job to the generated UpsertJob params, so every write path
// (ingest, telegram extraction, link import) shares one mapping instead of re-listing
// the columns. It owns the DERIVED columns too — content_hash and role_fingerprint are
// computed here rather than bolted on by each caller afterwards, so a write path cannot
// persist a posting missing either one by forgetting a step. A posted time supplied
// outside the deterministic derivation therefore belongs on the Draft, before this
// mapping runs: content_hash fingerprints posted_at, so setting it on the returned
// struct would fingerprint a value other than the one written.
// Enrichment columns are deliberately excluded — SetJobEnrichment owns those.
func (f Fields) UpsertParams() db.UpsertJobParams {
	p := db.UpsertJobParams{
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
	if f.SourceSalary != nil {
		p.SalaryMinSource = pgconv.Int4(f.SourceSalary.Min)
		p.SalaryMaxSource = pgconv.Int4(f.SourceSalary.Max)
		p.SalaryCurrencySource = f.SourceSalary.Currency
		p.SalaryPeriodSource = f.SourceSalary.Period
	}
	return withDerived(p)
}

// withDerived stamps the two fingerprints a persisted posting carries onto a mapped
// params struct. content_hash covers the indexed content (posted_at included), so the
// upsert can report whether a re-ingest changed anything searchable; role_fingerprint
// is the repost IDENTITY and deliberately excludes posted_at/url/slug, so a reposted
// role clusters with its original for the reality signal.
func withDerived(p db.UpsertJobParams) db.UpsertJobParams {
	p.ContentHash = pgtype.Text{String: jobhash.Of(p), Valid: true}
	p.RoleFingerprint = pgtype.Text{String: jobhash.RoleFingerprint(p), Valid: true}
	return p
}

// UpsertManualParams is the moderator-write analogue of UpsertParams: it maps the
// same Fields columns to the generated UpsertManualJob params, so the manual write
// path shares the one column mapping instead of re-listing them. It additionally
// seeds the salary_*_manual columns from the authoritative ManualSalary (nil when
// none) and stamps the moderator's id as created_by/updated_by.
//
// The derived columns are taken from UpsertParams rather than recomputed, so a
// hand-curated posting and a crawled one carry identical fingerprints for identical
// content — which is what lets the two cluster as one role instead of the manual copy
// sitting outside clustering.
func (f Fields) UpsertManualParams(actorID int64) db.UpsertManualJobParams {
	var salMin, salMax *int
	var salCurrency, salPeriod string
	if f.ManualSalary != nil {
		salMin, salMax = f.ManualSalary.Min, f.ManualSalary.Max
		salCurrency, salPeriod = f.ManualSalary.Currency, f.ManualSalary.Period
	}
	derived := f.UpsertParams()
	return db.UpsertManualJobParams{
		Source:      f.Source,
		ExternalID:  f.ExternalID,
		URL:         f.URL,
		Title:       f.Title,
		Company:     f.Company,
		CompanySlug: f.CompanySlug,
		Location:    f.Location,
		Remote:      f.Remote,
		Description: f.Description,
		PostedAt:    pgconv.Timestamptz(f.PostedAt),
		PublicSlug:  f.PublicSlug,
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

		SalaryMinManual:      pgconv.Int4(salMin),
		SalaryMaxManual:      pgconv.Int4(salMax),
		SalaryCurrencyManual: salCurrency,
		SalaryPeriodManual:   salPeriod,

		ContentHash:     derived.ContentHash,
		RoleFingerprint: derived.RoleFingerprint,

		CreatedBy: actorID,
		UpdatedBy: actorID,
	}
}

// UpdateManualParams is the moderator-EDIT analogue of UpsertManualParams: it maps the
// same Fields columns to the generated UpdateManualJob params, addressing the row by its
// public slug and stamping the acting moderator. It deliberately carries no salary or
// created_by — the edit query touches neither the manual salary nor the authorship of
// the original create.
//
// Like the other two mappings it owns the derived columns, and the edit is the write
// that most needs them: re-deriving facets from edited content is exactly when the
// fingerprints move.
func (f Fields) UpdateManualParams(slug string, actorID int64) db.UpdateManualJobParams {
	derived := f.UpsertParams()
	// jobhash.Of hashes p.PublicSlug, and UpsertParams() built derived from f.PublicSlug —
	// a slug job.New/jobderive.Derive just recomputed from the edited title/company. But
	// UpdateManualJob's own SQL comment is explicit that "the source identity (url/
	// external_id/public_slug) is deliberately NOT updatable here": the row's real
	// public_slug stays `slug`, the value already addressing it below. Hashing f.PublicSlug
	// would stamp content_hash with a slug value that never lands in the row, so
	// jobhash.OfRow (what a backfill/reconciliation pass calls, reading the row's ACTUAL
	// public_slug) could never reproduce it — the row would spuriously report "changed" on
	// every such pass even though nothing indexed actually moved. Re-derive against the
	// slug that is actually persisted; RoleFingerprint is unaffected (it deliberately
	// excludes public_slug, per its own doc comment), so this only changes ContentHash.
	derived.PublicSlug = slug
	derived = withDerived(derived)
	return db.UpdateManualJobParams{
		Title:       f.Title,
		Company:     f.Company,
		CompanySlug: f.CompanySlug,
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

		ContentHash:     derived.ContentHash,
		RoleFingerprint: derived.RoleFingerprint,

		UpdatedBy:  actorID,
		PublicSlug: slug,
	}
}

// InsertPrivateParams is the private-JD analogue of UpsertParams/UpsertManualParams: it
// maps the same Fields columns to the generated InsertPrivateJob params, stamping
// created_by. Unlike the other two mappings there is no updated_by or salary — a private
// job is never edited or given a manual salary, only created once per submission (see
// internal/privatejob).
func (f Fields) InsertPrivateParams(createdBy int64) db.InsertPrivateJobParams {
	derived := f.UpsertParams()
	return db.InsertPrivateJobParams{
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

		ContentHash:     derived.ContentHash,
		RoleFingerprint: derived.RoleFingerprint,

		CreatedBy: createdBy,
	}
}

// IsOpen reports whether the job is live (not soft-closed).
func (j Job) IsOpen() bool { return j.f.ClosedAt == nil }
