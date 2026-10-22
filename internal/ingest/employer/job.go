package employer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/platform/htmltext"
)

// jobSource is the source identity every vacancy this package writes carries — never
// editable per-posting, unlike moderation's Source (which defaults to "manual" but accepts
// the posting's real origin). See employer-job-authoring's spec.
const jobSource = job.EmployerSource

// ErrJobNotFound is a missing, not-owned, or not-employer-authored edit/close target.
var ErrJobNotFound = errors.New("employer: vacancy not found")

// ErrURLTaken is a create whose URL already identifies a vacancy a DIFFERENT employer
// account owns — refused rather than silently taken over. Re-creating under the same URL by
// its OWN owner is not an error; see CreateVacancy.
var ErrURLTaken = errors.New("employer: this URL is already used by another employer's vacancy")

// ApplyMode is how a candidate applies to a vacancy, chosen when it is created and fixed
// after: the mode is carried by the vacancy's external id (job.IsDirectApply), which is
// never recomputed.
type ApplyMode string

const (
	// ApplyExternal links out to the employer's own page — the vacancy's URL. The zero
	// value means this, so a caller that predates the choice keeps its behaviour.
	ApplyExternal ApplyMode = "external"
	// ApplyFreehire takes applications on freehire. The vacancy has no URL and carries
	// the employer's own questions.
	ApplyFreehire ApplyMode = "freehire"
)

// resolve maps the zero value to ApplyExternal and refuses a word it does not know.
func (m ApplyMode) resolve() (ApplyMode, error) {
	switch m {
	case "", ApplyExternal:
		return ApplyExternal, nil
	case ApplyFreehire:
		return ApplyFreehire, nil
	}
	return "", fmt.Errorf("%w: apply mode must be %q or %q", ErrInvalid, ApplyExternal, ApplyFreehire)
}

func modeOf(f job.Fields) ApplyMode {
	if job.IsDirectApply(f.Source, f.ExternalID) {
		return ApplyFreehire
	}
	return ApplyExternal
}

// questionsForm builds the stored form, mapping a refusal onto this package's ErrInvalid
// so the handler renders it as a 400 like every other validation failure.
func questionsForm(qs []applyform.EmployerQuestion) (applyform.Form, error) {
	form, err := applyform.FreehireForm(qs)
	if err != nil {
		return applyform.Form{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return form, nil
}

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
	// SaveForm stores (replacing) the questions of an on-site vacancy as its apply_forms
	// row. The vacancy is one this account already owns: a fresh mint, or a BySlug load.
	SaveForm(ctx context.Context, j job.Job, form applyform.Form) error
	// Update writes the full resulting row, scoped the same way BySlug reads.
	Update(ctx context.Context, actorID int64, slug string, f job.Fields) (job.Job, job.Extras, error)
	// Close soft-closes the vacancy (closed_reason='employer_closed'). Idempotent: closing
	// an already-closed vacancy is a no-op, not an error — ownership is the caller's job
	// (via a preceding BySlug), not this method's.
	Close(ctx context.Context, actorID int64, slug string) error
	// Reopen undoes a close this account made (closed_reason='employer_closed') and
	// reports whether there was one to undo; ownership is the caller's job, as for Close.
	Reopen(ctx context.Context, actorID int64, slug string) (bool, error)
}

// VacancyInput is the employer-supplied content for a new vacancy. Title is required, and
// so is URL in the external apply mode (Validate lives on moderation.CreateInput, which
// Create ultimately builds and delegates to — see CreateVacancy). In the freehire mode
// there is no URL and Questions are the employer's own. Company is deliberately absent: it
// is always the account's own locked company name, never request content.
type VacancyInput struct {
	ApplyMode   ApplyMode
	Questions   []applyform.EmployerQuestion
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
// ApplyMode may restate the vacancy's mode but never change it; Questions replaces the
// questions of an on-site vacancy and is refused on an external one.
type VacancyPatch struct {
	ApplyMode   *ApplyMode
	Questions   *[]applyform.EmployerQuestion
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
	if p.Description != nil {
		return requireDescription(*p.Description)
	}
	return nil
}

// requireDescription refuses a vacancy whose description reads as nothing once its markup
// is stripped. Search keeps such a posting out (search.DescriptionMissing), so an employer
// would publish it and never find it again; refusing it here is the honest version of that.
func requireDescription(description string) error {
	if strings.TrimSpace(htmltext.ToText(description)) == "" {
		return fmt.Errorf("%w: a description is required", ErrInvalid)
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
	mode, err := in.ApplyMode.resolve()
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	if err := requireDescription(in.Description); err != nil {
		return job.Job{}, job.Extras{}, err
	}
	acc, err := s.ActiveAccount(ctx, userID)
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	if mode == ApplyFreehire {
		return s.createOnSite(ctx, userID, acc, in)
	}
	if len(in.Questions) > 0 {
		return job.Job{}, job.Extras{}, fmt.Errorf("%w: only a vacancy taking applications on freehire has questions", ErrInvalid)
	}
	if strings.TrimSpace(in.URL) == "" {
		return job.Job{}, job.Extras{}, fmt.Errorf("%w: url is required", ErrInvalid)
	}

	owner, found, err := s.jobs.Owner(ctx, in.URL)
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	if found && owner != userID {
		return job.Job{}, job.Extras{}, ErrURLTaken
	}

	j, extras, err := s.minter.Create(ctx, userID, in.createInput(acc, in.URL, ""))
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

// createOnSite mints a vacancy that takes applications on freehire: a fresh
// freehire:<uuid> identity with no URL, then its questions. Neither of CreateVacancy's
// Owner checks applies — they guard a URL two employers could both name, and a UUID is
// one nobody else can. The questions are validated before the mint so a malformed form
// never leaves a vacancy behind. A failed form write after the mint closes the vacancy it
// just made: there is no URL to upsert on, so the employer's retry mints a second one,
// and the first must not stay live beside it asking none of the questions.
func (s *Service) createOnSite(ctx context.Context, userID int64, acc Account, in VacancyInput) (job.Job, job.Extras, error) {
	if in.URL != "" {
		return job.Job{}, job.Extras{}, fmt.Errorf("%w: a vacancy taking applications on freehire has no url", ErrInvalid)
	}
	form, err := questionsForm(in.Questions)
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	j, extras, err := s.minter.Create(ctx, userID, in.createInput(acc, "", job.DirectApplyPrefix+uuid.NewString()))
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	if err := s.jobs.SaveForm(ctx, j, form); err != nil {
		if closeErr := s.jobs.Close(ctx, userID, j.Fields().PublicSlug); closeErr != nil {
			return job.Job{}, job.Extras{}, fmt.Errorf("save questions: %w (and closing the vacancy failed: %v)", err, closeErr)
		}
		return job.Job{}, job.Extras{}, fmt.Errorf("save questions: %w", err)
	}
	return j, extras, nil
}

// createInput is the mint request for this account's vacancy: exactly one of url and
// externalID is set, by the apply mode.
func (in VacancyInput) createInput(acc Account, url, externalID string) moderation.CreateInput {
	return moderation.CreateInput{
		URL:         url,
		ExternalID:  externalID,
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
	}
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
	form, replaceForm, err := patch.formEdit(modeOf(curF))
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}

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
	j, extras, err := s.jobs.Update(ctx, userID, slug, f)
	if err != nil || !replaceForm {
		return j, extras, err
	}
	if err := s.jobs.SaveForm(ctx, j, form); err != nil {
		return job.Job{}, job.Extras{}, fmt.Errorf("save questions: %w", err)
	}
	return j, extras, nil
}

// formEdit checks the patch's apply mode and questions against the vacancy's current
// mode, and builds the replacement form when the patch carries one. Restating the current
// mode is allowed; changing it is not, since the mode is the vacancy's identity.
func (p VacancyPatch) formEdit(cur ApplyMode) (form applyform.Form, replace bool, err error) {
	if p.ApplyMode != nil {
		mode, err := p.ApplyMode.resolve()
		if err != nil {
			return applyform.Form{}, false, err
		}
		if mode != cur {
			return applyform.Form{}, false, fmt.Errorf("%w: the apply mode of a vacancy cannot change", ErrInvalid)
		}
	}
	if p.Questions == nil {
		return applyform.Form{}, false, nil
	}
	if cur != ApplyFreehire {
		return applyform.Form{}, false, fmt.Errorf("%w: only a vacancy taking applications on freehire has questions", ErrInvalid)
	}
	form, err = questionsForm(*p.Questions)
	if err != nil {
		return applyform.Form{}, false, err
	}
	return form, true, nil
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

// ReopenVacancy undoes userID's own close of their vacancy. Only a close the employer made
// is theirs to undo — one a moderator or the liveness probe made stays closed — and an
// open vacancy has nothing to undo; both are refused as invalid. It exists because a
// vacancy taking applications on freehire has no URL, so CreateVacancy's re-create-reopens
// path cannot reach it.
func (s *Service) ReopenVacancy(ctx context.Context, userID int64, slug string) error {
	if _, err := s.ActiveAccount(ctx, userID); err != nil {
		return err
	}
	if _, _, err := s.jobs.BySlug(ctx, userID, slug); err != nil {
		return err
	}
	reopened, err := s.jobs.Reopen(ctx, userID, slug)
	if err != nil {
		return err
	}
	if !reopened {
		return fmt.Errorf("%w: only a vacancy you closed can be reopened", ErrInvalid)
	}
	return nil
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
