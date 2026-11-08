package onboarding

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/maildaily"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Store is the slice of the database this feature reads and writes. Declared here
// rather than taking *db.Queries so the runner can be tested without Postgres.
type Store interface {
	maildaily.Store
	ListWelcomeCandidates(ctx context.Context, arg db.ListWelcomeCandidatesParams) ([]db.ListWelcomeCandidatesRow, error)
	ListAdvancedSearchCandidates(ctx context.Context, arg db.ListAdvancedSearchCandidatesParams) ([]db.ListAdvancedSearchCandidatesRow, error)
	ListNoAlertCandidates(ctx context.Context, arg db.ListNoAlertCandidatesParams) ([]db.ListNoAlertCandidatesRow, error)
	ListExtensionCandidates(ctx context.Context, arg db.ListExtensionCandidatesParams) ([]db.ListExtensionCandidatesRow, error)
	ListOpenSourceCandidates(ctx context.Context, arg db.ListOpenSourceCandidatesParams) ([]db.ListOpenSourceCandidatesRow, error)
	ListTailorCandidates(ctx context.Context, arg db.ListTailorCandidatesParams) ([]db.ListTailorCandidatesRow, error)
	ListBuddiesCandidates(ctx context.Context, arg db.ListBuddiesCandidatesParams) ([]db.ListBuddiesCandidatesRow, error)
	RecordOnboardingEmail(ctx context.Context, arg db.RecordOnboardingEmailParams) error
}

// Config bounds one pass.
type Config struct {
	// WindowDays is how far back a candidate scan looks. It is the safety rail on
	// the whole feature: the sequence was added long after the user table filled up,
	// and without a window the first run would greet every account ever created.
	WindowDays int32
	// AdvancedSearchAfterDays, NoAlertAfterDays, ExtensionAfterDays,
	// OpenSourceAfterDays, TailorAfterDays and BuddiesAfterDays are how long an account
	// must have existed (since the welcome greeting) before those steps are eligible. They must strictly increase — Run
	// iterates the steps in that order, and a later step racing ahead of an earlier one
	// is what the pacing exists to prevent.
	AdvancedSearchAfterDays int32
	NoAlertAfterDays        int32
	ExtensionAfterDays      int32
	OpenSourceAfterDays     int32
	TailorAfterDays         int32
	// BuddiesAfterDays is the interview-buddies letter's delay, on the same clock
	// and under the same strictly-increasing rule as the steps above.
	BuddiesAfterDays int32
	// BuddiesWindowDays is the buddies step's own window, used instead of WindowDays.
	// The letter goes out on day 14, which the shared 14-day window never reaches.
	// Widening the shared window instead would hand every earlier step accounts aged
	// 14-21 days that never got it: the tailor letter shipped after those accounts
	// aged out, so all of them would receive it at once, as would any late welcome.
	BuddiesWindowDays int32
	// MaxPerStep caps one pass per step. A cap is what keeps a backlog — or a bug
	// in the candidate query — from becoming one enormous send.
	MaxPerStep int32
}

// DefaultConfig is the shipped schedule: greet immediately, show the filter panel on
// day 3, ask about the missing alert on day 6, introduce the extension on day 8, talk
// about the project on day 10, introduce tailoring on day 12, introduce interview
// buddies on day 14.
func DefaultConfig() Config {
	return Config{
		WindowDays:              14,
		AdvancedSearchAfterDays: 3,
		NoAlertAfterDays:        6,
		ExtensionAfterDays:      8,
		OpenSourceAfterDays:     10,
		TailorAfterDays:         12,
		BuddiesAfterDays:        14,
		BuddiesWindowDays:       21,
		MaxPerStep:              200,
	}
}

// Runner performs one pass over every step.
type Runner struct {
	store  Store
	mailer *Mailer
	claims *maildaily.Claimer
	cfg    Config
}

// New builds a Runner.
func New(store Store, mailer *Mailer, cfg Config) *Runner {
	return &Runner{store: store, mailer: mailer, claims: maildaily.New(store, maildaily.SourceOnboarding), cfg: cfg}
}

// Stats is what one pass did, per step.
type Stats struct {
	Sent   map[Step]int
	Failed map[Step]int
	// Deferred counts the letters left pending because another sender already took the
	// account's mail for the day. Not a failure: the step is found again on a later pass.
	Deferred map[Step]int
}

// Run sends every eligible mail in every step.
//
// A step whose candidate query fails aborts the pass — that is a broken query or a
// broken database, and continuing would only produce more of the same error. A
// single failed *send*, by contrast, is recorded and stepped over: one bad address
// must not stop the other 199 people in the batch.
func (r *Runner) Run(ctx context.Context) (Stats, error) {
	stats := Stats{Sent: map[Step]int{}, Failed: map[Step]int{}, Deferred: map[Step]int{}}

	for _, step := range []Step{StepWelcome, StepAdvancedSearch, StepNoAlert, StepExtension, StepOpenSource, StepTailor, StepBuddies} {
		recipients, err := r.candidates(ctx, step)
		if err != nil {
			return stats, fmt.Errorf("onboarding: listing %s candidates: %w", step, err)
		}
		for _, rec := range recipients {
			switch r.deliver(ctx, step, rec) {
			case outcomeSent:
				stats.Sent[step]++
			case outcomeDeferred:
				stats.Deferred[step]++
			case outcomeFailed:
				stats.Failed[step]++
			case outcomePaused:
				stats.Failed[step]++
				log.Printf("onboarding: ses is not sending for the account, ending the pass at %s; nothing was recorded for the rest", step)
				return stats, nil
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

	case StepAdvancedSearch:
		rows, err := r.store.ListAdvancedSearchCandidates(ctx, db.ListAdvancedSearchCandidatesParams{
			WindowDays: r.cfg.WindowDays,
			AfterDays:  r.cfg.AdvancedSearchAfterDays,
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

	case StepExtension:
		rows, err := r.store.ListExtensionCandidates(ctx, db.ListExtensionCandidatesParams{
			WindowDays: r.cfg.WindowDays,
			AfterDays:  r.cfg.ExtensionAfterDays,
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

	case StepTailor:
		rows, err := r.store.ListTailorCandidates(ctx, db.ListTailorCandidatesParams{
			WindowDays: r.cfg.WindowDays,
			AfterDays:  r.cfg.TailorAfterDays,
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

	case StepBuddies:
		rows, err := r.store.ListBuddiesCandidates(ctx, db.ListBuddiesCandidatesParams{
			WindowDays: r.cfg.BuddiesWindowDays,
			AfterDays:  r.cfg.BuddiesAfterDays,
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

// outcome is what happened to one recipient in one pass.
type outcome int

const (
	outcomeSent outcome = iota
	// outcomeDeferred: the account's day was taken by another sender, so the step is left
	// pending and no ledger row is written.
	outcomeDeferred
	outcomeFailed
	// outcomePaused: SES refused the whole account; the caller ends the pass.
	outcomePaused
)

// deliver claims the account's mail for the day, sends one mail and closes out the
// ledger row.
//
// The claim comes first (maildaily): an account that already got another optional letter
// today is deferred — nothing is sent and nothing is recorded, so the step is simply a
// candidate again tomorrow. A claim that cannot be recorded counts as a failure and sends
// nothing, since an unrecorded claim cannot bound anything.
//
// A failure of one address burns the (user, step) pair: the ledger is written with
// the error. Retrying an address that SES just rejected is how a sending domain
// accumulates bounces, and the sequence is a courtesy — not worth risking
// deliverability for every other mail the product sends. Re-arming is a manual DELETE.
// The daily claim is released all the same, so the failed attempt does not cost the
// account another sender's letter today.
//
// An account-level refusal (emailnotify.ErrSendingPaused) is the opposite case and
// writes no row: nothing was judged about this address, so burning its slot would
// lose the mail for good just because SES was paused. The next pass finds it again.
func (r *Runner) deliver(ctx context.Context, step Step, rec recipient) outcome {
	claim, ok, err := r.claims.Take(ctx, rec.userID)
	if err != nil {
		log.Printf("onboarding: %s to user %d: %v", step, rec.userID, err)
		return outcomeFailed
	}
	if !ok {
		return outcomeDeferred
	}

	sendErr := r.mailer.Send(ctx, step, rec.userID, rec.email)
	if errors.Is(sendErr, emailnotify.ErrSendingPaused) {
		r.release(ctx, claim)
		return outcomePaused
	}

	var errText string
	if sendErr != nil {
		errText = sendErr.Error()
		log.Printf("onboarding: %s to user %d failed: %v", step, rec.userID, sendErr)
		r.release(ctx, claim)
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
	if sendErr != nil {
		return outcomeFailed
	}
	return outcomeSent
}

func (r *Runner) release(ctx context.Context, claim maildaily.Claim) {
	if err := r.claims.Release(ctx, claim); err != nil {
		log.Printf("onboarding: %v", err)
	}
}
