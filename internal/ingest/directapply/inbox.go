package directapply

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/strelov1/freehire/internal/ingest/employer"
)

// Stage is an application's position in the employer's own hiring pipeline. Every
// delivered application starts at StageNew.
type Stage string

const (
	StageNew       Stage = "new"
	StageReviewing Stage = "reviewing"
	StageInterview Stage = "interview"
	StageOffer     Stage = "offer"
	StageHired     Stage = "hired"
	StageRejected  Stage = "rejected"
)

// Stages is the pipeline vocabulary in order; migration 0270's CHECK names the same six.
var Stages = []Stage{StageNew, StageReviewing, StageInterview, StageOffer, StageHired, StageRejected}

// check refuses (ErrInvalid) a stage outside Stages.
func (st Stage) check() error {
	if !slices.Contains(Stages, st) {
		return fmt.Errorf("%w: unknown stage %q", ErrInvalid, st)
	}
	return nil
}

// candidateStage is the stage the candidate's own record shows for each employer stage
// (a userjob stage). StageNew is "applied", what the record already reads on arrival, so a
// move back to it undoes a stage the employer set by mistake (userjob.EmployerMay lets the
// employer correct its own stage, and only its own). The candidate sees every move, and that
// is the point of the feature: an employer on freehire cannot leave an applicant guessing.
var candidateStage = map[Stage]string{
	StageNew:       "applied",
	StageReviewing: "screening",
	StageInterview: "interview",
	StageOffer:     "offer",
	StageHired:     "accepted",
	StageRejected:  "rejected",
}

// Notifies reports whether moving an application to st emails the candidate. Only the
// moves a candidate acts on do; "reviewing" shows in their tracking without a letter, and a
// hire was agreed with them before the employer records it.
func (st Stage) Notifies() bool {
	return st == StageInterview || st == StageOffer || st == StageRejected
}

// maxMessage bounds the employer's note to the candidate, like the candidate's own note.
const maxMessage = 2000

// InboxApplication is one delivered application as the employer's inbox shows it.
type InboxApplication struct {
	ID             int64
	JobSlug        string
	JobTitle       string
	CandidateEmail string
	CVKind         string
	// HasCV is false for an application sent before its CV was kept; that CV reached the
	// employer by email only.
	HasCV          bool
	Note           string
	Answers        []Answer
	Stage          Stage
	StageChangedAt *time.Time
	SentAt         time.Time
}

// InboxFilter narrows the inbox. Empty JobSlug and Stage mean every vacancy and stage.
type InboxFilter struct {
	JobSlug string
	Stage   Stage
	Limit   int32
	Offset  int32
}

// InboxPage is one page of the inbox, with the count of applications at every stage under
// the same vacancy narrowing. Total is how many match the whole filter.
type InboxPage struct {
	Applications []InboxApplication
	Counts       map[Stage]int64
	Total        int64
}

// StageMove is what moving an application reports back to the use case.
type StageMove struct {
	// Changed is false when the application already stood at the stage.
	Changed        bool
	CandidateID    int64
	CandidateEmail string
	JobID          int64
	JobSlug        string
	JobTitle       string
	Company        string
}

// InboxRepository is the persistence the employer's inbox needs. Every method is scoped to
// the employer's own on-site vacancies and to delivered applications, and answers
// ErrNotFound for anything else.
type InboxRepository interface {
	List(ctx context.Context, employerID int64, f InboxFilter) ([]InboxApplication, error)
	Counts(ctx context.Context, employerID int64, jobSlug string) (map[Stage]int64, error)
	Get(ctx context.Context, employerID, id int64) (InboxApplication, error)
	// CV is the file the application sent. ErrNotFound also when no copy was kept.
	CV(ctx context.Context, employerID, id int64) (Attachment, error)
	SetStage(ctx context.Context, employerID, id int64, st Stage) (StageMove, error)
}

// StageMirror sets the candidate's own stage from the employer's move, when the
// candidate's record allows it. jobtracking.EmployerStages satisfies it.
type StageMirror interface {
	Mirror(ctx context.Context, userID, jobID int64, stage string) (bool, error)
}

// StatusNotifier emails the candidate that the employer moved their application.
// Implemented in internal/engage (emailnotify) and wired in internal/api/handler.
type StatusNotifier interface {
	SendStatusUpdate(ctx context.Context, m StatusMail) error
}

// StatusMail is the letter a candidate receives when the employer moves their application.
type StatusMail struct {
	To          string // the candidate
	ReplyTo     string // the employer account's confirmed work email
	CompanyName string
	JobTitle    string
	JobSlug     string
	Stage       Stage
	Message     string // the employer's own words, optional
}

// Inbox is the employer's side of on-site applications: reading the applications their
// vacancies received and moving each through their pipeline.
type Inbox struct {
	repo      InboxRepository
	employers Employers
	mirror    StageMirror
	notifier  StatusNotifier
	locks     ApplicationLocker
}

// ApplicationLocker runs fn while no other caller runs a function for the same
// application. ApplicationLocks satisfies it.
type ApplicationLocker interface {
	Serialize(ctx context.Context, id int64, fn func(context.Context) error) error
}

// NewInbox builds an Inbox. Every dependency is required.
func NewInbox(repo InboxRepository, employers Employers, mirror StageMirror, notifier StatusNotifier, locks ApplicationLocker) *Inbox {
	return &Inbox{repo: repo, employers: employers, mirror: mirror, notifier: notifier, locks: locks}
}

// Inbox page bounds.
const (
	DefaultInboxLimit = 50
	MaxInboxLimit     = 100
)

// List is one page of the delivered applications to employerID's vacancies, newest first.
func (in *Inbox) List(ctx context.Context, employerID int64, f InboxFilter) (InboxPage, error) {
	if _, err := in.employers.ActiveAccount(ctx, employerID); err != nil {
		return InboxPage{}, err
	}
	if f.Stage != "" {
		if err := f.Stage.check(); err != nil {
			return InboxPage{}, err
		}
	}
	switch {
	case f.Limit <= 0:
		f.Limit = DefaultInboxLimit
	case f.Limit > MaxInboxLimit:
		f.Limit = MaxInboxLimit
	}
	f.Offset = max(f.Offset, 0)

	counts, err := in.repo.Counts(ctx, employerID, f.JobSlug)
	if err != nil {
		return InboxPage{}, err
	}
	apps, err := in.repo.List(ctx, employerID, f)
	if err != nil {
		return InboxPage{}, err
	}
	page := InboxPage{Applications: apps, Counts: counts}
	if f.Stage != "" {
		page.Total = counts[f.Stage]
	} else {
		for _, n := range counts {
			page.Total += n
		}
	}
	return page, nil
}

// CV is the CV file an application sent, exactly as the employer received it.
func (in *Inbox) CV(ctx context.Context, employerID, id int64) (Attachment, error) {
	if _, err := in.employers.ActiveAccount(ctx, employerID); err != nil {
		return Attachment{}, err
	}
	return in.repo.CV(ctx, employerID, id)
}

// MoveResult is a moved application and whether the candidate was emailed about it.
type MoveResult struct {
	Application InboxApplication
	// Notified is true only when the letter went out. A stage that notifies but whose
	// letter failed is reported false, so the employer is never told the candidate knows
	// when they do not.
	Notified bool
}

// Move puts an application in the employer's pipeline at st. When that changes its stage,
// the candidate's own tracking follows (userjob.EmployerMay decides how far) and, for a
// stage that notifies, the candidate is emailed — with the employer's message when given,
// Reply-To the employer's work email. Neither follow-up can fail the move: the stage is
// the employer's record and is saved first; a mirror or mail failure is logged.
// A message is refused for a stage that does not notify, rather than silently dropped.
func (in *Inbox) Move(ctx context.Context, employerID, id int64, st Stage, message string) (MoveResult, error) {
	acc, err := in.employers.ActiveAccount(ctx, employerID)
	if err != nil {
		return MoveResult{}, err
	}
	if err := st.check(); err != nil {
		return MoveResult{}, err
	}
	message = strings.TrimSpace(message)
	if message != "" && !st.Notifies() {
		return MoveResult{}, fmt.Errorf("%w: a message to the candidate goes only with an interview, an offer or a rejection", ErrInvalid)
	}
	if utf8.RuneCountInString(message) > maxMessage {
		return MoveResult{}, fmt.Errorf("%w: the message is longer than %d characters", ErrInvalid, maxMessage)
	}

	// The whole move runs under the application's lock: two moves of one application never
	// interleave, so an older move cannot mirror or mail after a newer one has.
	var res MoveResult
	err = in.locks.Serialize(ctx, id, func(ctx context.Context) error {
		var err error
		res, err = in.move(ctx, acc, employerID, id, st, message)
		return err
	})
	return res, err
}

// move is Move's body, run under the application's lock.
func (in *Inbox) move(ctx context.Context, acc employer.Account, employerID, id int64, st Stage, message string) (MoveResult, error) {
	mv, err := in.repo.SetStage(ctx, employerID, id, st)
	if err != nil {
		return MoveResult{}, err
	}
	var notified bool
	if mv.Changed {
		if _, err := in.mirror.Mirror(ctx, mv.CandidateID, mv.JobID, candidateStage[st]); err != nil {
			log.Printf("directapply: candidate's tracking not updated for application %d: %v", id, err)
		}
		if st.Notifies() {
			err := in.notifier.SendStatusUpdate(ctx, StatusMail{
				To:          mv.CandidateEmail,
				ReplyTo:     acc.WorkEmail,
				CompanyName: mv.Company,
				JobTitle:    mv.JobTitle,
				JobSlug:     mv.JobSlug,
				Stage:       st,
				Message:     message,
			})
			if err != nil {
				log.Printf("directapply: candidate not emailed about application %d: %v", id, err)
			}
			notified = err == nil
		}
	}

	app, err := in.repo.Get(ctx, employerID, id)
	if err != nil {
		return MoveResult{}, err
	}
	return MoveResult{Application: app, Notified: notified}, nil
}
