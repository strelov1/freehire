// Package job defines the Job domain aggregate with a single guarded construction door. A
// Job is built only through New (fresh) or a Repository load (stored); its state is
// unexported, so no caller can bypass the deterministic facet derivation and a Job always
// carries facets consistent with its source fields.
package job

import (
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/job/jobhash"
	"github.com/strelov1/freehire/internal/job/reqextract"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// ErrInvalidDraft is returned by New when a draft lacks the identity a job
// requires: source, external id (together the dedup key), and a title.
var ErrInvalidDraft = errors.New("job: draft missing source, external id, or title")

// Salary is a stated compensation range in a currency's units over a period. A nil
// Min/Max means that bound was not stated. It carries the manual salary a submitter or
// moderator entered by hand, which takes precedence over the LLM-enriched salary.
type Salary struct {
	Min      *int
	Max      *int
	Currency string
	Period   string
}

// Draft is the source-agnostic input to New: the raw posting fields before derivation. The
// embedded jobderive.Input carries what derivation reads; Draft adds what it never touches
// (URL, remote flag, posted time, manual salary). The caller resolves Source/ExternalID
// (the dedup identity) so this package stays free of the source registry.
type Draft struct {
	jobderive.Input

	URL                 string
	Remote              bool
	PostedAt            *time.Time
	ApplicationDeadline *time.Time
	// Team is the department or team the source files the posting under, verbatim; trimmed by New.
	Team string

	ManualSalary *Salary

	// CanonicalCompanySlug overrides the derived company slug with the one a merge elected
	// (company_slug_aliases). Only the ingest pipeline fills it, because only it has the
	// registry and jobderive must stay pure. It rides on the Draft, not a setter, so no
	// caller can re-key a company; that decision belongs to the merge worker.
	CanonicalCompanySlug string

	// BodyFromCatalogue marks a Description the pipeline took from the stored row because the
	// crawl brought none (a failed detail fetch). It was not READ, so the write must not
	// stamp hydrated_at, which would drop a stale body from BODY_REFRESH_DAYS' re-read set.
	BodyFromCatalogue bool
}

// Fields is the readable projection of a Job, a plain DTO for the wire projection and
// search indexing. It can be constructed freely but is not a Job and cannot be persisted.
type Fields struct {
	Source     string
	ExternalID string
	URL        string
	// ApplicationURL is an independently verified destination. URL remains the source
	// posting for identity and liveness; ingestion never overwrites this destination.
	ApplicationURL      string
	Title               string
	Company             string
	CompanySlug         string
	PublicSlug          string
	Location            string
	Remote              bool
	Description         string
	PostedAt            *time.Time
	ApplicationDeadline *time.Time
	// Team: see Draft.Team. Empty when the source states none.
	Team string
	// BodyFromCatalogue: see Draft.BodyFromCatalogue.
	BodyFromCatalogue bool

	// deterministic dictionary facets
	Countries []string
	Regions   []string
	Cities    []string
	Metros    []string
	WorkMode  string
	Skills    []string
	Seniority string
	Category  string
	// IsTech is the tri-state technical signal (nil = unknown), derived from title and
	// category. See jobderive.Derived.IsTech.
	IsTech *bool
	// RequiresClearance marks a posting that states a government security-clearance
	// requirement (nil = not stated). Only true or nil; see jobderive.Derived.RequiresClearance.
	RequiresClearance *bool
	// ViaRecruiter marks a posting that states it is published by an intermediary on a
	// client's behalf (nil = not stated).
	ViaRecruiter *bool
	// Equity is a vocab.EquityValues member, or "" for not stated.
	Equity string
	// ClearanceLevels names the clearance levels stated (vocab.ClearanceLevelValues);
	// non-empty exactly when RequiresClearance is set.
	ClearanceLevels []string
	// Perks are the benefits the description states (vocab.PerkValues); empty means none
	// mentioned.
	Perks []string
	// Engagement names the legal terms the posting engages on (vocab.EngagementValues:
	// IR35, W-2, B2B...); empty means not stated.
	Engagement []string

	// synthetic enrichment facets (deterministic stand-ins)
	PostingLanguage    string
	EmploymentType     string
	EducationLevel     string
	EnglishLevel       string
	ExperienceYearsMin *int

	// ManualSalary is the manual salary (nil when none). Never derived; it takes
	// precedence over the enriched salary.
	ManualSalary *Salary
	// SourceSalary is the ATS's own structured salary (nil when none stated). It wins over
	// the LLM-derived guess but never over ManualSalary.
	SourceSalary *Salary

	// lifecycle + enrichment provenance (0 = unenriched)
	ClosedAt          *time.Time
	EnrichmentVersion int32

	// Read-only projection fields, populated by a repository load and zero on a fresh New.
	// ID is the persistence id (the domain identity is Source+ExternalID); ManuallyAdded is
	// moderator-authored provenance (created_by set); Enrichment is the RAW LLM payload
	// before any dict fold. None of these are part of the write surface.
	ID            int64
	ManuallyAdded bool
	// Private marks a posting a single user pasted in, visible only to them (jobs.is_private,
	// written by InsertPrivateJob; see internal/job/privatejob). It stays off the write
	// surface (the query that wrote the row decides it) and off the wire shape (jobview).
	Private    bool
	Enrichment enrich.Enrichment
	// RequirementsDerived is the posting's own requirements read from its description markup
	// (internal/job/reqextract), nil when none. The projection folds it into
	// enrichment.requirements when the model stated none. Read-only: UpsertParams derives its
	// own copy from the description, so a caller cannot persist a list that disagrees.
	RequirementsDerived []enrich.Requirement
	EnrichedAt          *time.Time
	CreatedAt           *time.Time
	UpdatedAt           *time.Time
	// LastSeenAt is when a re-crawl last confirmed this posting live, the freshest "still
	// open" evidence, used to estimate a rolling JobPosting.validThrough.
	LastSeenAt *time.Time
}

// Job is the domain aggregate. Its state is unexported; obtain one via New or a Repository
// load, and read it via Fields.
type Job struct {
	f Fields
}

// New constructs a fresh, open, unenriched Job from a draft, running the deterministic
// derivation (slugs and facets). It is the single construction door for a new posting.
func New(d Draft) (Job, error) {
	if d.Source == "" || d.ExternalID == "" || d.Title == "" {
		return Job{}, ErrInvalidDraft
	}
	// Strip a coordinate tail from the location before derivation and storage, as
	// pipeline.normalizeJob does.
	in := d.Input
	in.Location = normalize.CleanLocation(in.Location)
	der := jobderive.Derive(in)
	if d.CanonicalCompanySlug != "" {
		der.CompanySlug = d.CanonicalCompanySlug
	}
	return Job{f: Fields{
		Source:              d.Source,
		ExternalID:          d.ExternalID,
		URL:                 d.URL,
		Title:               d.Title,
		Company:             d.Company,
		CompanySlug:         der.CompanySlug,
		PublicSlug:          der.PublicSlug,
		Location:            in.Location,
		Remote:              d.Remote,
		Description:         d.Description,
		BodyFromCatalogue:   d.BodyFromCatalogue,
		PostedAt:            d.PostedAt,
		ApplicationDeadline: d.ApplicationDeadline,
		Team:                strings.TrimSpace(d.Team),

		Countries: der.Countries,
		Regions:   der.Regions,
		Cities:    der.Cities,
		Metros:    der.Metros,
		WorkMode:  der.WorkMode,
		Skills:    der.Skills,
		Seniority: der.Seniority,
		Category:  der.Category,
		IsTech:    der.IsTech,

		RequiresClearance: der.RequiresClearance,
		ViaRecruiter:      der.ViaRecruiter,
		Equity:            der.Equity,
		ClearanceLevels:   der.ClearanceLevels,
		Perks:             der.Perks,
		Engagement:        der.Engagement,

		PostingLanguage:    der.PostingLanguage,
		EmploymentType:     der.EmploymentType,
		EducationLevel:     der.EducationLevel,
		EnglishLevel:       der.EnglishLevel,
		ExperienceYearsMin: der.ExperienceYearsMin,

		// Manual salary is carried verbatim, never derived.
		ManualSalary: d.ManualSalary,
		SourceSalary: salaryFromParts(der.SalaryMin, der.SalaryMax, der.SalaryCurrency, der.SalaryPeriod),
	}}, nil
}

// salaryFromParts packages salary parts into a *Salary, or nil when neither bound is present.
func salaryFromParts(min, max *int, currency, period string) *Salary {
	if min == nil && max == nil {
		return nil
	}
	return &Salary{Min: min, Max: max, Currency: currency, Period: period}
}

// Fields returns the readable projection of the aggregate. Slice and pointer
// fields alias the aggregate's; callers treat the result as read-only.
func (j Job) Fields() Fields { return j.f }

// UpsertParams maps a job to the generated UpsertJob params, the one column mapping every
// write path shares. It computes the derived columns (content_hash, role_fingerprint) so no
// path can forget them. A posted time from outside derivation belongs on the Draft:
// content_hash fingerprints posted_at, so setting it on the returned struct would hash a
// value other than the one written. Enrichment columns are excluded; SetJobEnrichment owns them.
func (f Fields) UpsertParams() db.UpsertJobParams {
	p := db.UpsertJobParams{
		Source:              f.Source,
		ExternalID:          f.ExternalID,
		URL:                 f.URL,
		Title:               f.Title,
		Company:             f.Company,
		CompanySlug:         f.CompanySlug,
		PublicSlug:          f.PublicSlug,
		Location:            f.Location,
		Remote:              f.Remote,
		Description:         f.Description,
		BodyFromCatalogue:   f.BodyFromCatalogue,
		PostedAt:            pgconv.Timestamptz(f.PostedAt),
		ApplicationDeadline: pgconv.Date(f.ApplicationDeadline),
		Team:                f.Team,
		Countries:           f.Countries,
		Regions:             f.Regions,
		Cities:              f.Cities,
		Metros:              f.Metros,
		WorkMode:            f.WorkMode,
		Skills:              f.Skills,
		Seniority:           f.Seniority,
		Category:            f.Category,
		IsTech:              pgconv.Bool(f.IsTech),

		RequiresClearance: pgconv.Bool(f.RequiresClearance),
		ViaRecruiter:      pgconv.Bool(f.ViaRecruiter),
		Equity:            pgconv.Text(f.Equity),
		ClearanceLevels:   f.ClearanceLevels,
		Perks:             f.Perks,
		Engagement:        f.Engagement,

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

// withDerived stamps content_hash, role_fingerprint and requirements_derived onto a mapped
// params struct. content_hash covers the indexed content (posted_at included), so the upsert
// can report whether a re-ingest changed anything searchable; role_fingerprint is the repost
// identity and excludes posted_at/url/slug, so a reposted role clusters with its original.
// requirements_derived is a pure function of the raw fields, not enrichment (no model call).
func withDerived(p db.UpsertJobParams) db.UpsertJobParams {
	p.ContentHash = pgtype.Text{String: jobhash.Of(p), Valid: true}
	p.RoleFingerprint = pgtype.Text{String: jobhash.RoleFingerprint(p), Valid: true}
	p.RequirementsDerived = reqextract.DeriveJSON(p.Description)
	return p
}

// UpsertManualParams is the moderator-write analogue of UpsertParams, mapping to
// UpsertManualJob params. It also seeds the salary_*_manual columns from ManualSalary and
// stamps the moderator's id as created_by/updated_by.
//
// Derived columns come from UpsertParams, so a hand-curated and a crawled posting with
// identical content share fingerprints and cluster as one role.
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
		Metros:      f.Metros,
		WorkMode:    f.WorkMode,
		Skills:      f.Skills,
		Seniority:   f.Seniority,
		Category:    f.Category,
		IsTech:      pgconv.Bool(f.IsTech),

		RequiresClearance: pgconv.Bool(f.RequiresClearance),
		ViaRecruiter:      pgconv.Bool(f.ViaRecruiter),
		Equity:            pgconv.Text(f.Equity),
		ClearanceLevels:   f.ClearanceLevels,
		Perks:             f.Perks,
		Engagement:        f.Engagement,

		PostingLanguage:    f.PostingLanguage,
		EmploymentType:     f.EmploymentType,
		EducationLevel:     f.EducationLevel,
		EnglishLevel:       f.EnglishLevel,
		ExperienceYearsMin: pgconv.Int4(f.ExperienceYearsMin),

		SalaryMinManual:      pgconv.Int4(salMin),
		SalaryMaxManual:      pgconv.Int4(salMax),
		SalaryCurrencyManual: salCurrency,
		SalaryPeriodManual:   salPeriod,

		ContentHash:         derived.ContentHash,
		RoleFingerprint:     derived.RoleFingerprint,
		RequirementsDerived: derived.RequirementsDerived,

		CreatedBy: actorID,
		UpdatedBy: actorID,
	}
}

// UpdateManualParams is the moderator-EDIT analogue of UpsertManualParams, mapping to
// UpdateManualJob params addressed by public slug. It carries no salary or created_by; the
// edit query touches neither. Re-deriving from edited content is when fingerprints move.
func (f Fields) UpdateManualParams(slug string, actorID int64) db.UpdateManualJobParams {
	derived := f.UpsertParams()
	// UpdateManualJob does not update public_slug, so the row keeps `slug`. Hashing the
	// recomputed f.PublicSlug would stamp a content_hash that jobhash.OfRow (which reads the
	// row's actual slug) could never reproduce, so backfills would flag the row changed
	// forever. Re-derive against the persisted slug; only ContentHash changes.
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
		Metros:      f.Metros,
		WorkMode:    f.WorkMode,
		Skills:      f.Skills,
		Seniority:   f.Seniority,
		Category:    f.Category,
		IsTech:      pgconv.Bool(f.IsTech),

		RequiresClearance: pgconv.Bool(f.RequiresClearance),
		ViaRecruiter:      pgconv.Bool(f.ViaRecruiter),
		Equity:            pgconv.Text(f.Equity),
		ClearanceLevels:   f.ClearanceLevels,
		Perks:             f.Perks,
		Engagement:        f.Engagement,

		PostingLanguage:    f.PostingLanguage,
		EmploymentType:     f.EmploymentType,
		EducationLevel:     f.EducationLevel,
		EnglishLevel:       f.EnglishLevel,
		ExperienceYearsMin: pgconv.Int4(f.ExperienceYearsMin),

		ContentHash:         derived.ContentHash,
		RoleFingerprint:     derived.RoleFingerprint,
		RequirementsDerived: derived.RequirementsDerived,

		UpdatedBy:  actorID,
		PublicSlug: slug,
	}
}

// UpdateEmployerParams is the employer-self-service analogue of UpdateManualParams, with
// the same slug reasoning, targeting UpdateEmployerJob's narrower statement (WHERE
// created_by = actor_id AND source = 'employer'). UpdateManualJob is not safe to reuse here.
func (f Fields) UpdateEmployerParams(slug string, actorID int64) db.UpdateEmployerJobParams {
	derived := f.UpsertParams()
	derived.PublicSlug = slug
	derived = withDerived(derived)
	return db.UpdateEmployerJobParams{
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
		Metros:      f.Metros,
		WorkMode:    f.WorkMode,
		Skills:      f.Skills,
		Seniority:   f.Seniority,
		Category:    f.Category,
		IsTech:      pgconv.Bool(f.IsTech),

		RequiresClearance: pgconv.Bool(f.RequiresClearance),
		ViaRecruiter:      pgconv.Bool(f.ViaRecruiter),
		Equity:            pgconv.Text(f.Equity),
		ClearanceLevels:   f.ClearanceLevels,
		Perks:             f.Perks,
		Engagement:        f.Engagement,

		PostingLanguage:    f.PostingLanguage,
		EmploymentType:     f.EmploymentType,
		EducationLevel:     f.EducationLevel,
		EnglishLevel:       f.EnglishLevel,
		ExperienceYearsMin: pgconv.Int4(f.ExperienceYearsMin),

		ContentHash:         derived.ContentHash,
		RoleFingerprint:     derived.RoleFingerprint,
		RequirementsDerived: derived.RequirementsDerived,

		UpdatedBy:  actorID,
		PublicSlug: slug,
		ActorID:    actorID,
	}
}

// InsertPrivateParams is the private-JD analogue of UpsertParams, mapping to InsertPrivateJob
// params and stamping created_by. There is no updated_by or salary: a private job is only
// created once per submission (see internal/job/privatejob).
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
		Metros:      f.Metros,
		WorkMode:    f.WorkMode,
		Skills:      f.Skills,
		Seniority:   f.Seniority,
		Category:    f.Category,
		IsTech:      pgconv.Bool(f.IsTech),

		RequiresClearance: pgconv.Bool(f.RequiresClearance),
		ViaRecruiter:      pgconv.Bool(f.ViaRecruiter),
		Equity:            pgconv.Text(f.Equity),
		ClearanceLevels:   f.ClearanceLevels,
		Perks:             f.Perks,
		Engagement:        f.Engagement,

		PostingLanguage:    f.PostingLanguage,
		EmploymentType:     f.EmploymentType,
		EducationLevel:     f.EducationLevel,
		EnglishLevel:       f.EnglishLevel,
		ExperienceYearsMin: pgconv.Int4(f.ExperienceYearsMin),

		ContentHash:         derived.ContentHash,
		RoleFingerprint:     derived.RoleFingerprint,
		RequirementsDerived: derived.RequirementsDerived,

		CreatedBy: createdBy,
	}
}

// IsOpen reports whether the job is live (not soft-closed).
func (j Job) IsOpen() bool { return j.f.ClosedAt == nil }
