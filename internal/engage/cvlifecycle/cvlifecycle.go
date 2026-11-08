// Package cvlifecycle sends up to three letters to a free user whose CV has parsed, until
// they pay: what an ATS makes of the CV, what tailoring would add to that score, and an
// offer for the first month of Pro.
//
// cmd/cv-lifecycle-mail is the only caller. The letters are the founder's (a human
// Reply-To, first person), optional (they obey the news-email switch and the shared
// one-letter-a-day claim in internal/engage/maildaily) and measured: a stable tenth of
// accounts is enrolled and never mailed, so the effect on paying can be read against
// people who got nothing. AGENTS.md has the rules and the reports.
package cvlifecycle

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/maildaily"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Step is one letter of the sequence. The values are the ledger's `step` column, which a
// constraint there holds to 1..3.
type Step int16

const (
	StepATS       Step = 1
	StepPotential Step = 2
	StepOffer     Step = 3
)

// Steps is the sequence in order.
var Steps = []Step{StepATS, StepPotential, StepOffer}

// Campaign names the letter in SES engagement tracking.
func (s Step) Campaign() string { return fmt.Sprintf("cv-letter-%d", int16(s)) }

// Config is when each letter is due and how much one pass may do.
type Config struct {
	// AfterDays is how many days after the enrolled upload each letter becomes due. It
	// must be strictly increasing: a step is dropped once its successor is due.
	AfterDays [3]int32
	// EnrollWindowDays bounds how recent an upload must be to enroll the account, so the
	// first run does not pull in every historical upload.
	EnrollWindowDays int32
	// ObserveWindowDays is how long after enrollment the paying observation watches an
	// account, holdouts and unsubscribed ones included.
	ObserveWindowDays int32
	// EnrollPerRun caps one pass's enrollments; MaxPerRun caps its letters.
	EnrollPerRun int32
	MaxPerRun    int32
}

// DefaultConfig is the schedule the product runs: letters on days 1, 6 and 13.
func DefaultConfig() Config {
	return Config{
		AfterDays:         [3]int32{1, 6, 13},
		EnrollWindowDays:  14,
		ObserveWindowDays: 90,
		EnrollPerRun:      2000,
		MaxPerRun:         200,
	}
}

func (c Config) validate() error {
	if c.AfterDays[0] < 0 || c.AfterDays[0] >= c.AfterDays[1] || c.AfterDays[1] >= c.AfterDays[2] {
		return fmt.Errorf("cvlifecycle: AfterDays %v must be non-negative and strictly increasing", c.AfterDays)
	}
	if c.EnrollWindowDays <= 0 || c.ObserveWindowDays <= 0 || c.EnrollPerRun <= 0 || c.MaxPerRun <= 0 {
		return errors.New("cvlifecycle: windows and per-run caps must be positive")
	}
	return nil
}

// Store is the slice of the database the runner uses.
type Store interface {
	maildaily.Store
	EnrollCVLifecycleUsers(ctx context.Context, arg db.EnrollCVLifecycleUsersParams) (int64, error)
	CountCVLifecycleEnrollable(ctx context.Context, windowDays int32) (int64, error)
	ObserveCVLifecyclePaid(ctx context.Context, windowDays int32) (int64, error)
	ObserveCVLifecycleEntitled(ctx context.Context, windowDays int32) (int64, error)
	ExitCVLifecycleUnsubscribed(ctx context.Context) (int64, error)
	ListCVLifecycleDue(ctx context.Context, arg db.ListCVLifecycleDueParams) ([]db.ListCVLifecycleDueRow, error)
	ClaimCVLifecycleSend(ctx context.Context, arg db.ClaimCVLifecycleSendParams) (int64, error)
	RecordCVLifecycleSendError(ctx context.Context, arg db.RecordCVLifecycleSendErrorParams) error
	ReleaseCVLifecycleSend(ctx context.Context, arg db.ReleaseCVLifecycleSendParams) error
	PruneMailDayClaims(ctx context.Context) (int64, error)
}

// Composer builds what one letter says about one account. ErrSkip (wrapped) means the
// letter does not apply to this account now; any other error is an input that could not be
// read. Either way the step stays pending.
type Composer interface {
	Compose(ctx context.Context, step Step, userID int64) (Letter, error)
}

// Sender delivers a composed letter.
type Sender interface {
	Send(ctx context.Context, letter Letter, userID int64, to string) error
}

// Stats is what one pass did.
type Stats struct {
	Enrolled, Paid, Entitled, Unsubscribed, Pruned int
	// Sent, Failed, Skipped and Deferred are per step. Failed burned the step. Skipped
	// left it pending because the letter could not be built or the account stopped being
	// eligible; Deferred left it pending because another sender held the account's day.
	Sent, Failed, Skipped, Deferred map[Step]int
	// Errors counts work that could not be done for a reason that is not about one
	// account's eligibility: a letter input that could not be read, a database error.
	Errors int
	// Due is what a dry run found, per step.
	Due map[Step]int
	// Enrollable is what a dry run says would enroll.
	Enrollable int
	// Paused is set when SES refused the whole account and the pass stopped early.
	Paused bool
}

func newStats() Stats {
	return Stats{
		Sent: map[Step]int{}, Failed: map[Step]int{}, Skipped: map[Step]int{},
		Deferred: map[Step]int{}, Due: map[Step]int{},
	}
}

// Runner performs one pass.
type Runner struct {
	store    Store
	composer Composer
	sender   Sender
	claims   *maildaily.Claimer
	cfg      Config
}

// New builds a Runner. It refuses a schedule that could send a step after its successor.
func New(store Store, composer Composer, sender Sender, cfg Config) (*Runner, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &Runner{
		store: store, composer: composer, sender: sender,
		claims: maildaily.New(store, maildaily.SourceCVLifecycle), cfg: cfg,
	}, nil
}

// Run does one pass: observe who started paying, enroll new accounts, retire the ones that
// unsubscribed, then send each due letter. A failing bookkeeping statement aborts the pass,
// since sending on top of an unknown enrollment state is how a paying user gets an offer.
func (r *Runner) Run(ctx context.Context) (Stats, error) {
	stats := newStats()
	if err := r.bookkeep(ctx, &stats); err != nil {
		return stats, err
	}
	for _, step := range Steps {
		if stats.Paused {
			break
		}
		if err := r.runStep(ctx, step, &stats); err != nil {
			return stats, err
		}
	}
	return stats, nil
}

// DryRun counts what Run would do and writes nothing: no enrollment, no observation, no
// claim, no letter. Accounts that are not enrolled yet are reported as Enrollable only.
func (r *Runner) DryRun(ctx context.Context) (Stats, error) {
	stats := newStats()
	n, err := r.store.CountCVLifecycleEnrollable(ctx, r.cfg.EnrollWindowDays)
	if err != nil {
		return stats, fmt.Errorf("cvlifecycle: counting enrollable accounts: %w", err)
	}
	stats.Enrollable = int(n)
	for _, step := range Steps {
		var after int64
		for {
			rows, err := r.listDue(ctx, step, after)
			if err != nil {
				return stats, err
			}
			for _, row := range rows {
				stats.Due[step]++
				after = row.UserID
			}
			if len(rows) < int(r.pageSize()) {
				break
			}
		}
	}
	return stats, nil
}

// bookkeep runs the statements that decide who is in the sequence. The paying observation
// comes first so an account that has just paid is retired before anything is sent to it.
func (r *Runner) bookkeep(ctx context.Context, stats *Stats) error {
	steps := []struct {
		name string
		into *int
		run  func() (int64, error)
	}{
		{"observing paid", &stats.Paid, func() (int64, error) { return r.store.ObserveCVLifecyclePaid(ctx, r.cfg.ObserveWindowDays) }},
		{"observing entitled", &stats.Entitled, func() (int64, error) { return r.store.ObserveCVLifecycleEntitled(ctx, r.cfg.ObserveWindowDays) }},
		{"enrolling", &stats.Enrolled, func() (int64, error) {
			return r.store.EnrollCVLifecycleUsers(ctx, db.EnrollCVLifecycleUsersParams{
				WindowDays: r.cfg.EnrollWindowDays, MaxRows: r.cfg.EnrollPerRun,
			})
		}},
		{"retiring unsubscribed", &stats.Unsubscribed, func() (int64, error) { return r.store.ExitCVLifecycleUnsubscribed(ctx) }},
		{"pruning day claims", &stats.Pruned, func() (int64, error) { return r.store.PruneMailDayClaims(ctx) }},
	}
	for _, s := range steps {
		n, err := s.run()
		if err != nil {
			return fmt.Errorf("cvlifecycle: %s: %w", s.name, err)
		}
		*s.into = int(n)
	}
	return nil
}

func (r *Runner) pageSize() int32 { return r.cfg.MaxPerRun }

func (r *Runner) listDue(ctx context.Context, step Step, afterUser int64) ([]db.ListCVLifecycleDueRow, error) {
	rows, err := r.store.ListCVLifecycleDue(ctx, db.ListCVLifecycleDueParams{
		AfterDays:     r.afterDays(step),
		NextAfterDays: r.nextAfterDays(step),
		Step:          int16(step),
		AfterUserID:   afterUser,
		MaxRows:       r.pageSize(),
	})
	if err != nil {
		return nil, fmt.Errorf("cvlifecycle: listing step %d: %w", step, err)
	}
	return rows, nil
}

func (r *Runner) afterDays(step Step) int32 { return r.cfg.AfterDays[step-1] }

// nextAfterDays is the successor's offset, NULL for the last step.
func (r *Runner) nextAfterDays(step Step) pgtype.Int4 {
	if int(step) >= len(r.cfg.AfterDays) {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: r.cfg.AfterDays[step], Valid: true}
}

// runStep pages through the accounts a letter is due for until the pass has sent MaxPerRun
// letters or the list is exhausted. Skipped accounts do not spend the budget.
func (r *Runner) runStep(ctx context.Context, step Step, stats *Stats) error {
	budget := r.cfg.MaxPerRun - int32(attempted(stats))
	var after int64
	for budget > 0 {
		rows, err := r.listDue(ctx, step, after)
		if err != nil {
			return err
		}
		for _, row := range rows {
			after = row.UserID
			switch r.deliver(ctx, step, row, stats) {
			case outcomePaused:
				stats.Paused = true
				return nil
			case outcomeSent, outcomeFailed:
				budget--
			}
			if budget == 0 {
				return nil
			}
		}
		if len(rows) < int(r.pageSize()) {
			return nil
		}
	}
	return nil
}

func attempted(stats *Stats) int {
	n := 0
	for _, step := range Steps {
		n += stats.Sent[step] + stats.Failed[step]
	}
	return n
}

type outcome int

const (
	outcomeSkipped outcome = iota
	outcomeSent
	outcomeFailed
	outcomePaused
)

// deliver composes, claims and sends one letter.
//
// The order is the point. The letter is built first, so an input that cannot be read costs
// nothing. Then the step row is claimed by a statement that re-checks eligibility, then the
// account's day, and only then is SES called. The step row therefore exists before any mail
// can leave, which is what stops a repeated or concurrent run from sending a step twice.
//
//   - A claim that is not won (not eligible any more, or the day is held) leaves the step
//     pending; a step row written for a lost day is taken back.
//   - SES refusing the whole account (emailnotify.ErrSendingPaused) gives both claims back
//     and ends the pass: nothing was judged about this address.
//   - Any other send failure burns the step with its error and gives the day back.
//   - A crash after the claims keeps both, which fails toward fewer letters.
func (r *Runner) deliver(ctx context.Context, step Step, row db.ListCVLifecycleDueRow, stats *Stats) outcome {
	letter, err := r.composer.Compose(ctx, step, row.UserID)
	if err != nil {
		if !errors.Is(err, ErrSkip) {
			stats.Errors++
			log.Printf("cvlifecycle: step %d for user %d: %v", step, row.UserID, err)
		}
		stats.Skipped[step]++
		return outcomeSkipped
	}

	if _, err := r.store.ClaimCVLifecycleSend(ctx, db.ClaimCVLifecycleSendParams{
		Step: int16(step), UserID: row.UserID,
		AfterDays: r.afterDays(step), NextAfterDays: r.nextAfterDays(step),
	}); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			stats.Errors++
			log.Printf("cvlifecycle: claiming step %d for user %d: %v", step, row.UserID, err)
		}
		stats.Skipped[step]++
		return outcomeSkipped
	}

	claim, ok, err := r.claims.Take(ctx, row.UserID)
	if err != nil || !ok {
		r.releaseStep(ctx, step, row.UserID)
		if err != nil {
			stats.Errors++
			log.Printf("cvlifecycle: step %d for user %d: %v", step, row.UserID, err)
			stats.Skipped[step]++
		} else {
			stats.Deferred[step]++
		}
		return outcomeSkipped
	}

	sendErr := r.sender.Send(ctx, letter, row.UserID, row.Email)
	if errors.Is(sendErr, emailnotify.ErrSendingPaused) {
		r.releaseDay(ctx, claim)
		r.releaseStep(ctx, step, row.UserID)
		return outcomePaused
	}
	if sendErr != nil {
		stats.Failed[step]++
		log.Printf("cvlifecycle: step %d to user %d failed: %v", step, row.UserID, sendErr)
		if err := r.store.RecordCVLifecycleSendError(ctx, db.RecordCVLifecycleSendErrorParams{
			Error: sendErr.Error(), UserID: row.UserID, Step: int16(step),
		}); err != nil {
			log.Printf("cvlifecycle: recording the failure of step %d for user %d: %v", step, row.UserID, err)
		}
		r.releaseDay(ctx, claim)
		return outcomeFailed
	}
	stats.Sent[step]++
	return outcomeSent
}

func (r *Runner) releaseStep(ctx context.Context, step Step, userID int64) {
	if err := r.store.ReleaseCVLifecycleSend(ctx, db.ReleaseCVLifecycleSendParams{UserID: userID, Step: int16(step)}); err != nil {
		log.Printf("cvlifecycle: releasing step %d for user %d: %v", step, userID, err)
	}
}

func (r *Runner) releaseDay(ctx context.Context, claim maildaily.Claim) {
	if err := r.claims.Release(ctx, claim); err != nil {
		log.Printf("cvlifecycle: %v", err)
	}
}
