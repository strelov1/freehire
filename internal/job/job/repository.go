package job

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// Extras is the read-only projection data on the jobs row that is not part of the Job
// aggregate's invariants: engagement counters (from user_jobs) and collection slugs
// (from the company). Zero on a fresh New; populated only by FromRow. It lives here, not
// in jobview, so a load can return it without the domain importing the wire layer.
type Extras struct {
	ViewCount     int32
	AppliedCount  int32
	UpvoteCount   int32
	DownvoteCount int32
	Collections   []string
	// AIInterviewReports is how many people reported that this job's COMPANY screens with
	// an AI interviewer. Zero means nobody has, so the wire shape omits it.
	AIInterviewReports int32
}

// FromRow maps a persistence row to the domain Job plus its Extras. It never derives (a
// stored row already carries its facets), so it does not bypass New's invariant. It is the
// single place the domain depends on the db row shape.
func FromRow(r db.Job) (Job, Extras, error) {
	j, err := jobFromRow(r)
	if err != nil {
		return Job{}, Extras{}, err
	}
	return j, extrasFromRow(r), nil
}

// jobFromRow maps the aggregate-owned fields of a row into a domain Job, decoding the
// enrichment JSONB into the raw (pre-fold) Enrichment.
func jobFromRow(r db.Job) (Job, error) {
	var e enrich.Enrichment
	if len(r.Enrichment) > 0 {
		if err := json.Unmarshal(r.Enrichment, &e); err != nil {
			return Job{}, fmt.Errorf("job: decode enrichment for job %d: %w", r.ID, err)
		}
	}
	return Job{f: Fields{
		Source:              r.Source,
		ExternalID:          r.ExternalID,
		URL:                 r.URL,
		ApplicationURL:      r.ApplicationUrl,
		Title:               r.Title,
		Company:             r.Company,
		CompanySlug:         r.CompanySlug,
		PublicSlug:          r.PublicSlug,
		Location:            r.Location,
		Remote:              r.Remote,
		Description:         r.Description,
		PostedAt:            tsPtr(r.PostedAt),
		ApplicationDeadline: pgconv.DatePtr(r.ApplicationDeadline),
		Team:                r.Team,

		Countries: r.Countries,
		Regions:   r.Regions,
		Cities:    r.Cities,
		Metros:    r.Metros,
		WorkMode:  r.WorkMode,
		Skills:    r.Skills,
		Seniority: r.Seniority,
		Category:  r.Category,
		IsTech:    boolPtr(r.IsTech),

		RequiresClearance: boolPtr(r.RequiresClearance),
		ViaRecruiter:      boolPtr(r.ViaRecruiter),
		Equity:            pgconv.TextString(r.Equity),
		ClearanceLevels:   r.ClearanceLevels,
		Perks:             r.Perks,
		Engagement:        r.Engagement,

		PostingLanguage:    r.PostingLanguage,
		EmploymentType:     r.EmploymentType,
		EducationLevel:     r.EducationLevel,
		EnglishLevel:       r.EnglishLevel,
		ExperienceYearsMin: int4Ptr(r.ExperienceYearsMin),

		ManualSalary: manualSalaryFromRow(r),
		SourceSalary: sourceSalaryFromRow(r),

		ClosedAt:          tsPtr(r.ClosedAt),
		EnrichmentVersion: r.EnrichmentVersion,

		ID:                  r.ID,
		ManuallyAdded:       r.CreatedBy.Valid,
		Private:             r.IsPrivate,
		Enrichment:          e,
		RequirementsDerived: requirementsFromRow(r),
		EnrichedAt:          tsPtr(r.EnrichedAt),
		CreatedAt:           tsPtr(r.CreatedAt),
		UpdatedAt:           tsPtr(r.UpdatedAt),
		LastSeenAt:          tsPtr(r.LastSeenAt),
	}}, nil
}

// extrasFromRow pulls the read-only projection data off a row, apart from jobFromRow so
// the aggregate mapping stays free of non-aggregate fields.
func extrasFromRow(r db.Job) Extras {
	return Extras{
		ViewCount:     r.ViewCount,
		AppliedCount:  r.AppliedCount,
		UpvoteCount:   r.UpvoteCount,
		DownvoteCount: r.DownvoteCount,
		Collections:   r.Collections,

		AIInterviewReports: r.AiInterviewReports,
	}
}

// requirementsFromRow decodes the derived requirements column. An empty, pre-migration or
// undecodable value all come back nil ("no derived list"). A decode error is not fatal:
// the column is a display convenience, not worth failing the job read.
func requirementsFromRow(r db.Job) []enrich.Requirement {
	if len(r.RequirementsDerived) == 0 {
		return nil
	}
	var reqs []enrich.Requirement
	if err := json.Unmarshal(r.RequirementsDerived, &reqs); err != nil {
		return nil
	}
	return reqs
}

// manualSalaryFromRow projects the manual-salary columns onto a Salary, or nil when
// neither bound is set.
func manualSalaryFromRow(r db.Job) *Salary {
	if !r.SalaryMinManual.Valid && !r.SalaryMaxManual.Valid {
		return nil
	}
	return &Salary{
		Min:      int4Ptr(r.SalaryMinManual),
		Max:      int4Ptr(r.SalaryMaxManual),
		Currency: r.SalaryCurrencyManual,
		Period:   r.SalaryPeriodManual,
	}
}

func sourceSalaryFromRow(r db.Job) *Salary {
	if !r.SalaryMinSource.Valid && !r.SalaryMaxSource.Valid {
		return nil
	}
	return &Salary{
		Min:      int4Ptr(r.SalaryMinSource),
		Max:      int4Ptr(r.SalaryMaxSource),
		Currency: r.SalaryCurrencySource,
		Period:   r.SalaryPeriodSource,
	}
}

// tsPtr renders a nullable Postgres timestamp as *time.Time (nil when unset),
// keeping the aggregate free of pgtype.
func tsPtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}

// boolPtr renders a nullable Postgres bool as *bool (nil when unset), keeping the
// tri-state is_tech signal out of pgtype in the aggregate.
func boolPtr(b pgtype.Bool) *bool {
	if !b.Valid {
		return nil
	}
	v := b.Bool
	return &v
}

// int4Ptr renders a nullable Postgres int4 as *int (nil when unset).
func int4Ptr(n pgtype.Int4) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int32)
	return &v
}
