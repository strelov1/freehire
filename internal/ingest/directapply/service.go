package directapply

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/candidate/answerbank"
	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/ingest/employer"
)

// The refusals Apply can return besides ErrInvalid. The handler maps each to its status.
var (
	// ErrNotFound is a vacancy that does not take applications here (missing, private, or
	// an ordinary posting) or a CV the candidate does not own.
	ErrNotFound = errors.New("directapply: not found")
	// ErrClosed is a vacancy that has closed, or whose employer account is no longer active.
	ErrClosed = errors.New("directapply: this vacancy no longer takes applications")
	// ErrUnverified is a candidate whose email address is not verified.
	ErrUnverified = errors.New("directapply: confirm your email address before applying")
	// ErrAlreadyApplied is a vacancy this candidate's application already reached.
	ErrAlreadyApplied = errors.New("directapply: you have already applied to this vacancy")
	// ErrInProgress is an application another request of the same candidate is delivering
	// right now — a double click, or a client retrying a slow response.
	ErrInProgress = errors.New("directapply: this application is being sent")
	// ErrDailyLimit is a candidate who has sent today's allowance of applications.
	ErrDailyLimit = errors.New("directapply: you have reached today's limit of applications")
	// ErrDelivery is an application recorded but not delivered. It stays unsent, and
	// applying again retries it.
	ErrDelivery = errors.New("directapply: the application could not be delivered")
)

// Vacancy is what Apply needs to know about the posting applied to.
type Vacancy struct {
	ID    int64
	Slug  string
	Title string
	// EmployerID is the account that authored it (jobs.created_by).
	EmployerID int64
	Open       bool
}

// Candidate is the applying user's address and whether they proved they own it.
type Candidate struct {
	Email    string
	Verified bool
}

// Record is one application as stored before delivery.
type Record struct {
	JobID   int64
	UserID  int64
	CVID    uuid.UUID
	Note    string
	Answers []Answer
}

// Repository is the persistence this package needs.
type Repository interface {
	// Vacancy loads an open-or-closed on-site vacancy by slug; ErrNotFound for anything that
	// is not one (missing, private, or not job.IsDirectApply).
	Vacancy(ctx context.Context, slug string) (Vacancy, error)
	// Form is the vacancy's stored questions; a vacancy with no stored form asks none.
	Form(ctx context.Context, jobID int64) (applyform.Form, error)
	Candidate(ctx context.Context, userID int64) (Candidate, error)
	// Record writes the application and claims it for delivery, or replaces an unsent,
	// unclaimed one for the same vacancy. It returns recorded=false when nothing was
	// written — State tells why.
	Record(ctx context.Context, r Record, dailyLimit int64) (id int64, recorded bool, err error)
	// State is where userID's application to jobID stands.
	State(ctx context.Context, jobID, userID int64) (State, error)
	// MarkSent stamps an application delivered and drops its claim.
	MarkSent(ctx context.Context, id int64) error
	// Release drops the delivery claim after a failed send.
	Release(ctx context.Context, id int64) error
}

// State is where one candidate's application to one vacancy stands.
type State int

const (
	// StateNone: no application.
	StateNone State = iota
	// StateUnsent: recorded, not delivered, and no delivery in flight.
	StateUnsent
	// StateSending: a delivery holds the claim right now.
	StateSending
	// StateSent: the employer has it.
	StateSent
)

// Employers answers whether a vacancy's author may still receive applications, and where.
// employer.Service satisfies it.
type Employers interface {
	ActiveAccount(ctx context.Context, userID int64) (employer.Account, error)
}

// CVs renders a CV the candidate owns to PDF; ErrNotFound when they own no such CV.
type CVs interface {
	PDF(ctx context.Context, userID int64, cvID uuid.UUID) ([]byte, error)
}

// Mailer delivers an application to the employer. Implemented in internal/engage
// (emailnotify) and wired in internal/api/handler — this block cannot import engage.
type Mailer interface {
	SendApplication(ctx context.Context, m Mail) error
}

// Mail is everything the employer receives.
type Mail struct {
	To             string // the employer account's confirmed work email
	CandidateEmail string // Reply-To
	CompanyName    string
	JobTitle       string
	JobSlug        string
	Answers        []Answer
	Note           string
	CV             []byte // PDF
}

// Tracker records the application in the candidate's own tracking.
type Tracker interface {
	MarkApplied(ctx context.Context, userID int64, slug, source string) error
}

// AnswerBank remembers what the candidate answered, so the next form is prefilled.
type AnswerBank interface {
	Save(ctx context.Context, userID int64, question, answer string, by answerbank.Provenance) error
}

// Application is what the candidate submits.
type Application struct {
	CVID    uuid.UUID
	Answers map[string]string // question id -> answer
	Note    string
}

// Service is the on-site application use case.
type Service struct {
	repo       Repository
	employers  Employers
	cvs        CVs
	mailer     Mailer
	tracker    Tracker
	bank       AnswerBank
	dailyLimit int64
}

// New builds a Service. Every dependency is required; the handler declines to build one
// (and answers 501) when PDF rendering or mail is not configured.
func New(repo Repository, employers Employers, cvs CVs, mailer Mailer, tracker Tracker, bank AnswerBank, dailyLimit int64) *Service {
	return &Service{repo: repo, employers: employers, cvs: cvs, mailer: mailer, tracker: tracker, bank: bank, dailyLimit: dailyLimit}
}

// Apply records userID's application to the on-site vacancy at slug and delivers it to the
// employer. The application is recorded — and claimed for delivery — before the email and
// stamped sent only after it, so a failed send is never reported as sent: it returns
// ErrDelivery, releases its claim, stays unsent, and the candidate applying again replaces
// its content and retries it. The claim is what stops a second, overlapping request from
// emailing the employer twice (design.md, decision 4).
// Tracking and the answer bank are updated only once the employer has it, and neither can
// fail the application.
func (s *Service) Apply(ctx context.Context, userID int64, slug string, app Application) error {
	v, err := s.repo.Vacancy(ctx, slug)
	if err != nil {
		return err
	}
	if v.EmployerID == userID {
		// An employer emailing themselves their own vacancy's application is never meant;
		// answered as for any posting that takes no application from this caller.
		return ErrNotFound
	}
	if !v.Open {
		return ErrClosed
	}
	acc, err := s.employers.ActiveAccount(ctx, v.EmployerID)
	if errors.Is(err, employer.ErrNotActive) || errors.Is(err, employer.ErrNotFound) {
		return ErrClosed
	}
	if err != nil {
		return err
	}
	cand, err := s.repo.Candidate(ctx, userID)
	if err != nil {
		return err
	}
	if !cand.Verified {
		return ErrUnverified
	}

	form, err := s.repo.Form(ctx, v.ID)
	if err != nil {
		return err
	}
	answers, err := checkAnswers(form, app.Answers)
	if err != nil {
		return err
	}
	note, err := checkNote(app.Note)
	if err != nil {
		return err
	}
	pdf, err := s.cvs.PDF(ctx, userID, app.CVID)
	if err != nil {
		return err
	}

	id, recorded, err := s.repo.Record(ctx, Record{JobID: v.ID, UserID: userID, CVID: app.CVID, Note: note, Answers: answers}, s.dailyLimit)
	if err != nil {
		return err
	}
	if !recorded {
		state, err := s.repo.State(ctx, v.ID, userID)
		if err != nil {
			return err
		}
		switch state {
		case StateSent:
			return ErrAlreadyApplied
		case StateSending:
			return ErrInProgress
		}
		return ErrDailyLimit
	}

	if err := s.mailer.SendApplication(ctx, Mail{
		To:             acc.WorkEmail,
		CandidateEmail: cand.Email,
		CompanyName:    acc.CompanyName,
		JobTitle:       v.Title,
		JobSlug:        v.Slug,
		Answers:        answers,
		Note:           note,
		CV:             pdf,
	}); err != nil {
		if relErr := s.repo.Release(ctx, id); relErr != nil {
			slog.WarnContext(ctx, "directapply: delivery claim not released", "application_id", id, "err", relErr)
		}
		return fmt.Errorf("%w: %w", ErrDelivery, err)
	}
	if err := s.repo.MarkSent(ctx, id); err != nil {
		// Delivered but not stamped: a retry would send it a second time. Reported, since
		// the employer HAS it — telling the candidate it failed would invite exactly that.
		slog.ErrorContext(ctx, "directapply: delivered application not stamped sent", "application_id", id, "err", err)
	}

	if err := s.tracker.MarkApplied(ctx, userID, v.Slug, appevent.SourceUser); err != nil {
		slog.WarnContext(ctx, "directapply: tracking not updated", "job_id", v.ID, "err", err)
	}
	s.bankAnswers(ctx, userID, answers)
	return nil
}

// bankAnswers saves each answer as candidate-provided. A boolean is banked as the words a
// person would write, since the bank also prefills ATS forms that ask in prose. A question
// the bank cannot key, or a failed save, is skipped: the bank is a convenience.
func (s *Service) bankAnswers(ctx context.Context, userID int64, answers []Answer) {
	for _, a := range answers {
		value := a.Answer
		if a.Type == applyform.TypeBoolean {
			value = yesNo[a.Answer]
		}
		err := s.bank.Save(ctx, userID, a.Label, value, answerbank.AuthorCandidate)
		if err != nil && !errors.Is(err, answerbank.ErrUnkeyable) {
			slog.WarnContext(ctx, "directapply: answer not banked", "question_id", a.ID, "err", err)
		}
	}
}

// yesNo is how a boolean answer reads to a person.
var yesNo = map[string]string{"true": "Yes", "false": "No"}
