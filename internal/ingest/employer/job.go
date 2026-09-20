package employer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/platform/htmltext"
)

// jobSource is the source identity every vacancy this package writes carries — never
// editable per-posting, unlike moderation's Source (which defaults to "manual" but accepts
// the posting's real origin). See employer-job-authoring's spec.
const jobSource = "employer"

// ErrJobNotFound is a missing, not-owned, or not-employer-authored edit/close target.
var ErrJobNotFound = errors.New("employer: vacancy not found")

// ErrURLTaken is a create whose URL already identifies a vacancy a DIFFERENT employer
// account owns — refused rather than silently taken over. Re-creating under the same URL by
// its OWN owner is not an error; see CreateVacancy.
var ErrURLTaken = errors.New("employer: this URL is already used by another employer's vacancy")

// Minter mints a live vacancy from validated content. moderation.Service satisfies it —
// the same seam internal/ingest/submission already uses to mint an approved submission,
// reused here so Create's validation, derivation, upsert, and enrichment enqueue are not
// duplicated for a third write path.
type Minter interface {
	Create(ctx context.Context, actorID int64, in moderation.CreateInput) (job.Job, job.Extras, error)
}

// JobRepository is the persistence contract for an employer's own vacancies, expressed in
// the job package's domain types.
type JobRepository interface {
	// Owner reports who created the job at (jobSource, externalID), if any — the
	// pre-Create check that stops a different employer taking over a posting via a
	// colliding URL (see ErrURLTaken).
	Owner(ctx context.Context, externalID string) (createdBy int64, found bool, err error)
	// BySlug loads a vacancy by its public slug, scoped to actorID's own jobSource-authored
	// postings; ErrJobNotFound for anything else (missing, another owner's, another
	// source's, or private).
	BySlug(ctx context.Context, actorID int64, slug string) (job.Job, job.Extras, error)
	// ListMine returns every vacancy actorID has created through this path, newest first —
	// the dashboard's own list, open and closed both.
	ListMine(ctx context.Context, actorID int64) ([]job.Job, []job.Extras, error)
	// Update writes the full resulting row, scoped the same way BySlug reads.
	Update(ctx context.Context, actorID int64, slug string, f job.Fields) (job.Job, job.Extras, error)
	// Close soft-closes the vacancy (closed_reason='employer_closed'). Idempotent: closing
	// an already-closed vacancy is a no-op, not an error — ownership is the caller's job
	// (via a preceding BySlug), not this method's.
	Close(ctx context.Context, actorID int64, slug string) error
}

// VacancyInput is the employer-supplied content for a new vacancy. URL and Title are
// required (Validate lives on moderation.CreateInput, which Create ultimately builds and
// delegates to — see CreateVacancy). Company is deliberately absent: it is always the
// account's own locked company name, never request content.
type VacancyInput struct {
	URL         string
	Title       string
	Location    string
	Remote      bool
	Description string
	PostedAt    *time.Time

	Skills         []string
	Regions        []string
	Cities         []string
	WorkMode       string
	EmploymentType string
	Seniority      string
	SalaryMin      *int
	SalaryMax      *int
	SalaryCurrency string
	SalaryPeriod   string
}

// VacancyPatch is a partial edit: a nil field is left unchanged. URL and the company
// identity are not present — see employer-job-authoring's spec for why neither is editable.
type VacancyPatch struct {
	Title       *string
	Location    *string
	Remote      *bool
	Description *string
	PostedAt    *time.Time
}

func (p VacancyPatch) validate() error {
	if p.Title != nil && strings.TrimSpace(*p.Title) == "" {
		return fmt.Errorf("%w: title must not be empty", ErrInvalid)
	}
	return nil
}

// ListVacancies returns every vacancy userID has published through this path, newest first
// — the dashboard's own list. Gated on ActiveAccount like every other employer capability
// except MyAccount itself (a pending/revoked account has no vacancies to list yet, since
// CreateVacancy is ActiveAccount-gated too).
func (s *Service) ListVacancies(ctx context.Context, userID int64) ([]job.Job, []job.Extras, error) {
	if _, err := s.ActiveAccount(ctx, userID); err != nil {
		return nil, nil, err
	}
	return s.jobs.ListMine(ctx, userID)
}

// CreateVacancy publishes a new vacancy for userID's own claimed company, or idempotently
// updates/reopens it when in.URL already identifies a vacancy this SAME account owns (see
// employer-job-authoring's "re-creating reopens" requirement — UpsertManualJob's own
// ON CONFLICT clears closed_at, so no separate reopen path is needed). A URL already owned
// by a DIFFERENT employer account is refused as ErrURLTaken, never silently overwritten.
//
// The pre-check below and the mint that follows are NOT one atomic step — two employer
// accounts racing to create the same not-yet-existing URL can both pass it, and
// UpsertManualJob's ON CONFLICT never reassigns created_by, so the loser's write would
// silently overwrite the winner's content while still returning 201 as if the loser owned
// the row. The RE-CHECK after Create closes that: whichever request's write actually landed
// last reads back its own outcome and the other is told the truth (ErrURLTaken) instead of
// a false success. This does not stop the fleeting overwrite itself — a genuine fix needs
// the ownership check baked into the upsert's own WHERE clause, which UpsertManualJob
// cannot carry (moderation deliberately allows ANY moderator to edit ANY manual job) — but
// it does stop a caller ever being told they own a row that is actually someone else's,
// which is the failure that matters: every later PATCH/close from that account is scoped by
// created_by and would otherwise 404 with no explanation.
func (s *Service) CreateVacancy(ctx context.Context, userID int64, in VacancyInput) (job.Job, job.Extras, error) {
	acc, err := s.ActiveAccount(ctx, userID)
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}

	owner, found, err := s.jobs.Owner(ctx, in.URL)
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	if found && owner != userID {
		return job.Job{}, job.Extras{}, ErrURLTaken
	}

	j, extras, err := s.minter.Create(ctx, userID, moderation.CreateInput{
		URL:         in.URL,
		Source:      jobSource,
		Title:       in.Title,
		Company:     acc.CompanyName,
		Location:    in.Location,
		Remote:      in.Remote,
		Description: in.Description,
		PostedAt:    in.PostedAt,

		Skills:         in.Skills,
		Regions:        in.Regions,
		Cities:         in.Cities,
		WorkMode:       in.WorkMode,
		EmploymentType: in.EmploymentType,
		Seniority:      in.Seniority,
		SalaryMin:      in.SalaryMin,
		SalaryMax:      in.SalaryMax,
		SalaryCurrency: in.SalaryCurrency,
		SalaryPeriod:   in.SalaryPeriod,
	})
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}

	owner, found, err = s.jobs.Owner(ctx, in.URL)
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	if found && owner != userID {
		return job.Job{}, job.Extras{}, ErrURLTaken
	}
	return j, extras, nil
}

// UpdateVacancy loads userID's own vacancy, overlays the supplied (nil-means-unchanged)
// fields, and re-derives the deterministic facets from the merged content — the same shape
// moderation.Service.Update uses, but through JobRepository's actor-scoped read/write rather
// than moderation's "any manually-authored job" ones. The company identity, URL, and
// external id are never touched.
func (s *Service) UpdateVacancy(ctx context.Context, userID int64, slug string, patch VacancyPatch) (job.Job, job.Extras, error) {
	if err := patch.validate(); err != nil {
		return job.Job{}, job.Extras{}, err
	}
	if _, err := s.ActiveAccount(ctx, userID); err != nil {
		return job.Job{}, job.Extras{}, err
	}

	cur, _, err := s.jobs.BySlug(ctx, userID, slug)
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	curF := cur.Fields()

	title := stringOr(patch.Title, curF.Title)
	location := stringOr(patch.Location, curF.Location)
	// Sanitize a supplied description before persisting (stored XSS), matching every other
	// manual write path; re-sanitizing the already-clean current value is idempotent.
	description := htmltext.Sanitize(stringOr(patch.Description, curF.Description))
	remote := curF.Remote
	if patch.Remote != nil {
		remote = *patch.Remote
	}
	postedAt := curF.PostedAt
	if patch.PostedAt != nil {
		postedAt = patch.PostedAt
	}

	f, err := deriveEmployer(curF.ExternalID, title, curF.Company, location, description, remote)
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	f.Remote = remote
	f.PostedAt = postedAt
	return s.jobs.Update(ctx, userID, slug, f)
}

// CloseVacancy soft-closes userID's own vacancy. The ownership/existence check happens via
// BySlug before Close ever runs, so Close itself only has idempotency left to handle — an
// already-closed vacancy closes again with no error, matching every other closer in the
// catalogue (see job-lifecycle).
func (s *Service) CloseVacancy(ctx context.Context, userID int64, slug string) error {
	if _, err := s.ActiveAccount(ctx, userID); err != nil {
		return err
	}
	if _, _, err := s.jobs.BySlug(ctx, userID, slug); err != nil {
		return err
	}
	return s.jobs.Close(ctx, userID, slug)
}

// deriveEmployer builds the Job aggregate through the same shared primitives
// moderation.go's own private derive() wraps (job.New / jobderive.Input) — not that private
// function itself, which stays scoped to moderation's own CreateInput/UpdatePatch shapes.
// An edit carries no structured facet overrides: every facet re-derives from content, the
// same as moderation.Service.Update's own edit path.
func deriveEmployer(externalID, title, company, location, description string, remote bool) (job.Fields, error) {
	workMode := ""
	if remote {
		workMode = "remote"
	}
	j, err := job.New(job.Draft{
		Input: jobderive.Input{
			Source:      jobSource,
			ExternalID:  externalID,
			Title:       title,
			Company:     company,
			Location:    location,
			Description: description,
			WorkMode:    workMode,
		},
	})
	if err != nil {
		return job.Fields{}, err
	}
	return j.Fields(), nil
}

// stringOr returns *p when set, else fallback — the nil-means-unchanged merge, matching
// moderation.Service.Update's own helper of the same name.
func stringOr(p *string, fallback string) string {
	if p != nil {
		return *p
	}
	return fallback
}
