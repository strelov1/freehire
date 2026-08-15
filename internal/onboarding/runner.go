package onboarding

import (
	"context"
	"fmt"
	"log"

	"github.com/strelov1/freehire/internal/db"
)

// Store is the slice of the database this feature reads and writes. Declared here
// rather than taking *db.Queries so the runner can be tested without Postgres.
type Store interface {
	ListWelcomeCandidates(ctx context.Context, arg db.ListWelcomeCandidatesParams) ([]db.ListWelcomeCandidatesRow, error)
	ListNoAlertCandidates(ctx context.Context, arg db.ListNoAlertCandidatesParams) ([]db.ListNoAlertCandidatesRow, error)
	ListOpenSourceCandidates(ctx context.Context, arg db.ListOpenSourceCandidatesParams) ([]db.ListOpenSourceCandidatesRow, error)
	RecordOnboardingEmail(ctx context.Context, arg db.RecordOnboardingEmailParams) error
}

// Config bounds one pass.
type Config struct {
	// WindowDays is how far back a candidate scan looks. It is the safety rail on
	// the whole feature: the sequence was added long after the user table filled up,
	// and without a window the first run would greet every account ever created.
	WindowDays int32
	// NoAlertAfterDays and OpenSourceAfterDays are how long an account must have
	// existed before those steps are eligible.
	NoAlertAfterDays    int32
	OpenSourceAfterDays int32
	// MaxPerStep caps one pass per step. A cap is what keeps a backlog — or a bug
	// in the candidate query — from becoming one enormous send.
	MaxPerStep int32
}

// DefaultConfig is the shipped schedule: greet immediately, ask about the missing
// alert on day 3, talk about the project on day 10.
func DefaultConfig() Config {
	return Config{
		WindowDays:          14,
		NoAlertAfterDays:    3,
		OpenSourceAfterDays: 10,
		MaxPerStep:          200,
	}
}

// Runner performs one pass over all three steps.
type Runner struct {
	store  Store
	mailer *Mailer
	cfg    Config
}

// New builds a Runner.
func New(store Store, mailer *Mailer, cfg Config) *Runner {
	return &Runner{store: store, mailer: mailer, cfg: cfg}
}

// Stats is what one pass did, per step.
type Stats struct {
	Sent   map[Step]int
	Failed map[Step]int
}

// Run sends every eligible mail in all three steps.
//
// A step whose candidate query fails aborts the pass — that is a broken query or a
// broken database, and continuing would only produce more of the same error. A
// single failed *send*, by contrast, is recorded and stepped over: one bad address
// must not stop the other 199 people in the batch.
func (r *Runner) Run(ctx context.Context) (Stats, error) {
	stats := Stats{Sent: map[Step]int{}, Failed: map[Step]int{}}

	for _, step := range []Step{StepWelcome, StepNoAlert, StepOpenSource} {
		recipients, err := r.candidates(ctx, step)
		if err != nil {
			return stats, fmt.Errorf("onboarding: listing %s candidates: %w", step, err)
		}
		for _, rec := range recipients {
			if r.deliver(ctx, step, rec) {
				stats.Sent[step]++
			} else {
				stats.Failed[step]++
			}
		}
	}
	return stats, nil
}

// recipient is one person to mail.
type recipient struct {
	userID int64
	email  string
}

func (r *Runner) candidates(ctx context.Context, step Step) ([]recipient, error) {
	switch step {
	case StepWelcome:
		rows, err := r.store.ListWelcomeCandidates(ctx, db.ListWelcomeCandidatesParams{
			WindowDays: r.cfg.WindowDays,
			MaxRows:    r.cfg.MaxPerStep,
		})
		if err != nil {
			return nil, err
		}
		out := make([]recipient, 0, len(rows))
		for _, row := range rows {
			out = append(out, recipient{userID: row.ID, email: row.Email})
		}
		return out, nil

	case StepNoAlert:
		rows, err := r.store.ListNoAlertCandidates(ctx, db.ListNoAlertCandidatesParams{
			WindowDays: r.cfg.WindowDays,
			AfterDays:  r.cfg.NoAlertAfterDays,
			MaxRows:    r.cfg.MaxPerStep,
		})
		if err != nil {
			return nil, err
		}
		out := make([]recipient, 0, len(rows))
		for _, row := range rows {
			out = append(out, recipient{userID: row.ID, email: row.Email})
		}
		return out, nil

	case StepOpenSource:
		rows, err := r.store.ListOpenSourceCandidates(ctx, db.ListOpenSourceCandidatesParams{
			WindowDays: r.cfg.WindowDays,
			AfterDays:  r.cfg.OpenSourceAfterDays,
			MaxRows:    r.cfg.MaxPerStep,
		})
		if err != nil {
			return nil, err
		}
		out := make([]recipient, 0, len(rows))
		for _, row := range rows {
			out = append(out, recipient{userID: row.ID, email: row.Email})
		}
		return out, nil
	}
	return nil, fmt.Errorf("unknown step %q", step)
}

// deliver sends one mail and closes out the ledger row. It reports whether the send
// itself succeeded.
//
// The ledger is written either way, and that is the important part: a failed send
// still burns the (user, step) pair. Retrying an address that SES just rejected is
// how a sending domain accumulates bounces, and the sequence is a courtesy — not
// worth risking deliverability for every other mail the product sends. Re-arming is
// a manual DELETE.
func (r *Runner) deliver(ctx context.Context, step Step, rec recipient) bool {
	sendErr := r.mailer.Send(ctx, step, rec.email)

	var errText string
	if sendErr != nil {
		errText = sendErr.Error()
		log.Printf("onboarding: %s to user %d failed: %v", step, rec.userID, sendErr)
	}

	if err := r.store.RecordOnboardingEmail(ctx, db.RecordOnboardingEmailParams{
		UserID: rec.userID,
		Step:   string(step),
		Error:  errText,
	}); err != nil {
		// The mail went out but the ledger did not record it. Log loudly: the next
		// pass will send it again, and a duplicate greeting is the one visible
		// symptom this feature can produce.
		log.Printf("onboarding: recording %s for user %d failed (mail may repeat): %v", step, rec.userID, err)
	}
	return sendErr == nil
}
