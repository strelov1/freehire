package directapply

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/candidate/answerbank"
	"github.com/strelov1/freehire/internal/dict/answertopic"
	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/ingest/employer"
)

// The refusals Apply can return besides ErrInvalid. Like ErrInvalid, their text is
// user-facing — the handler renders it as is — so it carries no package prefix.
var (
	// ErrNotFound is a vacancy that does not take applications here (missing, private, or
	// an ordinary posting) or a CV the candidate does not own.
	ErrNotFound = errors.New("not found")
	// ErrClosed is a vacancy that has closed, or whose employer account is no longer active.
	ErrClosed = errors.New("this vacancy no longer takes applications")
	// ErrUnverified is a candidate whose email address is not verified.
	ErrUnverified = errors.New("confirm your email address before applying")
	// ErrAlreadyApplied is a vacancy this candidate's application already reached.
	ErrAlreadyApplied = errors.New("you have already applied to this vacancy")
	// ErrInProgress is an application another request of the same candidate is delivering
	// right now — a double click, or a client retrying a slow response.
	ErrInProgress = errors.New("your application is being sent")
	// ErrDailyLimit is a candidate who has sent today's allowance of applications.
	ErrDailyLimit = errors.New("you have reached today's limit of applications")
	// ErrDelivery is an application recorded but not delivered. It stays unsent, and
	// applying again retries it. It wraps the transport's own error, which can name the
	// employer's address: render ErrDelivery's text, never the wrapped one.
	ErrDelivery = errors.New("the application could not be delivered — try again")
)

// Vacancy is what Apply needs to know about the posting applied to.
type Vacancy struct {
	ID      int64
	Slug    string
	Title   string
	Company string
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

// refusal is why an application in this state cannot be sent now, or nil when it can.
func (st State) refusal() error {
	switch st {
	case StateSent:
		return ErrAlreadyApplied
	case StateSending:
		return ErrInProgress
	}
	return nil
}

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
// answerbank.Store satisfies it.
type AnswerBank interface {
	Save(ctx context.Context, userID int64, question, answer string, by answerbank.Provenance) error
	// Sendable is the candidate's own answers by topic — never an agent's suggestion.
	Sendable(ctx context.Context, userID int64) (map[string]string, error)
}

// FormView is what the job page needs to show the application form.
type FormView struct {
	CompanyName string
	Questions   []applyform.Field
	// Open is false once the vacancy closed or its employer account stopped being active.
	Open bool
	// Applied and Prefill are the signed-in candidate's own; empty for anyone else.
	Applied bool
	Prefill map[string]string // question id -> answer, in the form Apply accepts
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
	acc, receiving, err := s.receivingEmployer(ctx, v.EmployerID)
	if err != nil {
		return err
	}
	if !receiving {
		return ErrClosed
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
	// A cheap read before the expensive render: a vacancy this candidate already applied
	// to, or is applying to right now, is refused without rendering a PDF it would discard.
	state, err := s.repo.State(ctx, v.ID, userID)
	if err != nil {
		return err
	}
	if err := state.refusal(); err != nil {
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
		if err := state.refusal(); err != nil {
			return err
		}
		return ErrDailyLimit
	}

	if err := s.mailer.SendApplication(ctx, Mail{
		To:             acc.WorkEmail,
		CandidateEmail: cand.Email,
		CompanyName:    v.Company,
		JobTitle:       v.Title,
		JobSlug:        v.Slug,
		Answers:        answers,
		Note:           note,
		CV:             pdf,
	}); err != nil {
		if relErr := s.repo.Release(ctx, id); relErr != nil {
			log.Printf("directapply: delivery claim on application %d not released: %v", id, relErr)
		}
		return fmt.Errorf("%w: %w", ErrDelivery, err)
	}
	if err := s.repo.MarkSent(ctx, id); err != nil {
		// Delivered but not stamped: a retry would send it a second time. Reported, since
		// the employer HAS it — telling the candidate it failed would invite exactly that.
		log.Printf("directapply: application %d was delivered but not stamped sent: %v", id, err)
	}

	if err := s.tracker.MarkApplied(ctx, userID, v.Slug, appevent.SourceUser); err != nil {
		log.Printf("directapply: tracking not updated for job %d: %v", v.ID, err)
	}
	s.bankAnswers(ctx, userID, answers)
	return nil
}

// receivingEmployer loads the account that authored a vacancy and reports whether it may
// still receive applications. A revoked or deleted account is not an error here: it means
// the vacancy no longer takes applications.
func (s *Service) receivingEmployer(ctx context.Context, userID int64) (employer.Account, bool, error) {
	acc, err := s.employers.ActiveAccount(ctx, userID)
	if errors.Is(err, employer.ErrNotActive) || errors.Is(err, employer.ErrNotFound) {
		return employer.Account{}, false, nil
	}
	if err != nil {
		return employer.Account{}, false, err
	}
	return acc, true, nil
}

// Form is the on-site vacancy's application form as userID sees it (0 for a visitor who is
// not signed in): the questions, whether it still takes applications, and for a candidate
// whether they already applied and what their answer bank offers for each question.
func (s *Service) Form(ctx context.Context, userID int64, slug string) (FormView, error) {
	v, err := s.repo.Vacancy(ctx, slug)
	if err != nil {
		return FormView{}, err
	}
	form, err := s.repo.Form(ctx, v.ID)
	if err != nil {
		return FormView{}, err
	}
	open := v.Open
	if open {
		if _, open, err = s.receivingEmployer(ctx, v.EmployerID); err != nil {
			return FormView{}, err
		}
	}
	view := FormView{CompanyName: v.Company, Questions: form.Fields, Open: open}
	if userID == 0 || userID == v.EmployerID {
		return view, nil
	}

	state, err := s.repo.State(ctx, v.ID, userID)
	if err != nil {
		return FormView{}, err
	}
	view.Applied = state == StateSent
	banked, err := s.bank.Sendable(ctx, userID)
	if err != nil {
		// A prefill is a convenience; the form is still answerable without it.
		log.Printf("directapply: answer bank unreadable: %v", err)
		return view, nil
	}
	view.Prefill = prefill(form, banked)
	return view, nil
}

// prefill offers each question the candidate's banked answer to the same topic, in the
// shape Apply accepts: a yes/no question takes a banked "Yes"/"No", a list question takes a
// banked value only when it is one of its options, and a text question takes it as is.
func prefill(form applyform.Form, banked map[string]string) map[string]string {
	out := map[string]string{}
	for _, f := range form.Fields {
		topic, ok := answertopic.Of(f.Label)
		if !ok {
			continue
		}
		v := strings.TrimSpace(banked[topic])
		if v == "" {
			continue
		}
		var answer string
		switch f.Type {
		case applyform.TypeBoolean:
			answer = fromYesNo[strings.ToLower(v)]
		case applyform.TypeSelect:
			if slices.ContainsFunc(f.Options, func(o applyform.Option) bool { return o.Value == v }) {
				answer = v
			}
		default:
			answer = v
		}
		if answer != "" {
			out[f.ID] = answer
		}
	}
	return out
}

// fromYesNo reads a banked yes/no back into the form's boolean answer.
var fromYesNo = map[string]string{"yes": "true", "no": "false"}

// bankAnswers saves each answer as candidate-provided, as a person reads it. A question
// the bank cannot key, or a failed save, is skipped: the bank is a convenience.
func (s *Service) bankAnswers(ctx context.Context, userID int64, answers []Answer) {
	for _, a := range answers {
		err := s.bank.Save(ctx, userID, a.Label, a.Readable(), answerbank.AuthorCandidate)
		if err != nil && !errors.Is(err, answerbank.ErrUnkeyable) {
			log.Printf("directapply: answer to %s not banked: %v", a.ID, err)
		}
	}
}
