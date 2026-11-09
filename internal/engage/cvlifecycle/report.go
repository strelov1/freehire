package cvlifecycle

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/strelov1/freehire/internal/platform/db"
)

// PaidWindowDays is how long after a letter an account may start paying and still count
// as paying because of it.
const PaidWindowDays int32 = 14

// ReportStore is the slice of the database the report reads.
type ReportStore interface {
	CVLetterEngagement(ctx context.Context) ([]db.CVLetterEngagementRow, error)
	CVLetterPayments(ctx context.Context, arg db.CVLetterPaymentsParams) (db.CVLetterPaymentsRow, error)
	CVLetterHoldoutAtStep(ctx context.Context, arg db.CVLetterHoldoutAtStepParams) (db.CVLetterHoldoutAtStepRow, error)
	CVLetterGroupsFromEnrollment(ctx context.Context, windowDays int32) ([]db.CVLetterGroupsFromEnrollmentRow, error)
}

// Cohort is a count of accounts and how many of them started paying inside the window.
type Cohort struct {
	Accounts, Paid int64
}

// Rate is Paid/Accounts, and false when there is nobody to divide by.
func (c Cohort) Rate() (float64, bool) {
	if c.Accounts == 0 {
		return 0, false
	}
	return float64(c.Paid) / float64(c.Accounts), true
}

// LetterReport is one letter: what SES saw, and whether its recipients started paying.
type LetterReport struct {
	Step Step
	// Sent is every letter in the step ledger; Delivered, Clicked, Bounced and Complained
	// are SES's counts for the campaign. Opens are not tracked (see docs/agents/notifications.md).
	Sent, Delivered, Clicked, Bounced, Complained int64
	// Recipients are the letter's recipients whose window has closed, paid-within-window
	// is read from them. Control is the holdout on the same clock: unpaid when the letter
	// fell due, window closed.
	Recipients, Control Cohort
}

// Report answers whether the CV lifecycle letters cause payments.
type Report struct {
	WindowDays int32
	Letters    []LetterReport
	// Holdout and Mailed are measured from enrollment, each over accounts whose window
	// has closed. Mailed is every non-holdout account, whether or not a letter reached it.
	Holdout, Mailed Cohort
}

// BuildReport reads the figures. The step offsets come from cfg, so the control for a
// letter is anchored where the letter falls due.
func BuildReport(ctx context.Context, store ReportStore, cfg Config) (Report, error) {
	r := Report{WindowDays: PaidWindowDays}

	events, err := store.CVLetterEngagement(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("cvlifecycle: engagement: %w", err)
	}
	byCampaign := map[string]map[string]int64{}
	for _, e := range events {
		if byCampaign[e.Campaign] == nil {
			byCampaign[e.Campaign] = map[string]int64{}
		}
		byCampaign[e.Campaign][e.Type] = e.Events
	}

	for i, step := range Steps {
		pay, err := store.CVLetterPayments(ctx, db.CVLetterPaymentsParams{WindowDays: r.WindowDays, Step: int16(step)})
		if err != nil {
			return Report{}, fmt.Errorf("cvlifecycle: payments of step %d: %w", step, err)
		}
		control, err := store.CVLetterHoldoutAtStep(ctx, db.CVLetterHoldoutAtStepParams{AfterDays: cfg.AfterDays[i], WindowDays: r.WindowDays})
		if err != nil {
			return Report{}, fmt.Errorf("cvlifecycle: holdout at step %d: %w", step, err)
		}
		ev := byCampaign[step.Campaign()]
		r.Letters = append(r.Letters, LetterReport{
			Step:       step,
			Sent:       pay.Sent,
			Delivered:  ev["delivery"],
			Clicked:    ev["click"],
			Bounced:    ev["bounce"],
			Complained: ev["complaint"],
			Recipients: Cohort{Accounts: pay.Matured, Paid: pay.Paid},
			Control:    Cohort{Accounts: control.Accounts, Paid: control.Paid},
		})
	}

	groups, err := store.CVLetterGroupsFromEnrollment(ctx, r.WindowDays)
	if err != nil {
		return Report{}, fmt.Errorf("cvlifecycle: groups: %w", err)
	}
	for _, g := range groups {
		if g.Holdout {
			r.Holdout = Cohort{Accounts: g.Accounts, Paid: g.Paid}
		} else {
			r.Mailed = Cohort{Accounts: g.Accounts, Paid: g.Paid}
		}
	}
	return r, nil
}

func percent(c Cohort) string {
	rate, ok := c.Rate()
	if !ok {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", rate*100)
}

// Write prints the report as plain text.
func (r Report) Write(w io.Writer) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	// A tabwriter buffers; the first write error comes back from Flush.
	line := func(format string, args ...any) { _, _ = fmt.Fprintf(tw, format, args...) }
	line("LETTER\tSENT\tDELIVERED\tOPENED\tCLICKED\tBOUNCED\tCOMPLAINED\tPAID<=%dd\tHOLDOUT PAID<=%dd\n", r.WindowDays, r.WindowDays)
	for _, l := range r.Letters {
		line("%d\t%d\t%d\tn/a\t%d\t%d\t%d\t%d/%d (%s)\t%d/%d (%s)\n",
			l.Step, l.Sent, l.Delivered, l.Clicked, l.Bounced, l.Complained,
			l.Recipients.Paid, l.Recipients.Accounts, percent(l.Recipients),
			l.Control.Paid, l.Control.Accounts, percent(l.Control))
	}
	line("\nFROM ENROLLMENT\tACCOUNTS\tPAID<=%dd\n", r.WindowDays)
	line("holdout (no letters)\t%d\t%d (%s)\n", r.Holdout.Accounts, r.Holdout.Paid, percent(r.Holdout))
	line("mailed (all, letter or not)\t%d\t%d (%s)\n", r.Mailed.Accounts, r.Mailed.Paid, percent(r.Mailed))
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, `
Reading it:
- A letter's PAID and HOLDOUT PAID cover only accounts whose %d-day window has closed. The holdout
  column is the control on the letter's own clock: holdout accounts still unpaid when the letter
  fell due, counted over the %d days after. Compare those two, not the letter against the
  enrollment rows, which count the first %d days of an account's life.
- The enrollment rows are the whole-sequence comparison: every non-holdout account, mailed or not.
- A payer after letter 2 also sits inside letter 1's window when the letters are under %d days
  apart: the letters' PAID counts overlap and do not add up.
- OPENED is n/a: the configuration set publishes no Open event, by decision (open counts are
  inflated by mail privacy proxies and link scanners). Clicks may be inflated by scanners too.
- PAID is a Stripe or RevenueCat tier first seen live by the hourly observation, which can lag
  the purchase by up to an hour. App-store purchases that never reach the backend are invisible.
- SES counts keep the ledger's first sighting per message; events past 90 days are daily counts.
`, r.WindowDays, r.WindowDays, r.WindowDays, r.WindowDays)
	return err
}
