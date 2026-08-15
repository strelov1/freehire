// Package mailpreview renders every outgoing freehire email against fixed sample
// data, so the design can be reviewed without sending anything.
//
// It exists because email is the one surface with no dev server: the templates run
// inside cron workers, against real accounts, and the result lands in someone's
// inbox. Rendering them here turns "what does the password-reset mail look like?"
// into a file you can open.
//
// The samples feed two consumers: cmd/mail-preview writes them to disk for
// Storybook, and mailpreview_test.go asserts the committed files still match, so a
// template change that nobody re-previews fails the build rather than quietly
// leaving Storybook showing last month's design.
//
// Adding a mail: write a renderer and list it in `renderers`. Nothing else needs
// to change — the contact sheet, the Storybook gallery and the staleness test all
// read that list.
package mailpreview

import (
	"context"
	"fmt"

	"github.com/strelov1/freehire/internal/emailnotify"
	"github.com/strelov1/freehire/internal/mailtpl"
	"github.com/strelov1/freehire/internal/notify"
	"github.com/strelov1/freehire/internal/nudge"
	"github.com/strelov1/freehire/internal/onboarding"
	"github.com/strelov1/freehire/internal/referral"
	"github.com/strelov1/freehire/internal/reminder"
	"github.com/strelov1/freehire/internal/report"
)

// DefaultBaseURL is what the committed previews link to: the preview directory
// itself, not the production origin.
//
// Relative is the useful default because it makes the output self-contained — the
// logo travels with the files, so the previews render correctly opened straight off
// disk and inside Storybook, neither of which can reach a host that has not been
// deployed yet. The real URLs are still visible in each preview's plain-text panel,
// which is where you would check a link anyway.
//
// Pass -base=https://freehire.me to see the production URLs in the markup.
const DefaultBaseURL = "."

// Sample is one rendered mail.
type Sample struct {
	// Name is the file stem and the Storybook entry: kebab-case, unique.
	Name string
	// Title is how the mail is listed in Storybook.
	Title string
	// Subject is the line the recipient sees in their inbox.
	Subject string
	// HTML is the full rendered document, exactly as it would be sent: it follows
	// the reader's colour preference.
	HTML string
	// LightHTML and DarkHTML are the same document pinned to one scheme, so a
	// reviewer can see both without changing their own OS setting.
	LightHTML string
	DarkHTML  string
	// Text is the plain-text alternative, shown beside the HTML so the two can be
	// compared — a text body that has drifted from the HTML is invisible otherwise.
	Text string
}

// Samples renders every mail against baseURL. The order is the order they appear in
// Storybook: account mails first, then the recurring ones, then the rare ones.
func Samples(baseURL string) ([]Sample, error) {
	var out []Sample
	for _, r := range renderers {
		s, err := r(baseURL)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// renderers is the list of mails. Each drives the real notifier through the capture
// sender, so a sample is what production would send, not a hand-written copy of it.
var renderers = []func(string) (Sample, error){
	welcomeSample,
	noAlertSample,
	openSourceSample,
	verificationSample,
	passwordResetSample,
	digestSample,
	savedJobReminderSample,
	followUpNudgeSample,
	jobClosedNudgeSample,
	referralRequestSample,
	reportRemovedSample,
	reportDismissedSample,
}

// capture is a Sender that keeps the last message instead of delivering it. It
// satisfies every mail package's sender interface — they all declare the same
// method, locally, to avoid depending on the AWS graph.
type capture struct {
	subject, html, text string
}

func (c *capture) Send(_ context.Context, _, _, subject, htmlBody, textBody string) error {
	c.subject, c.html, c.text = subject, htmlBody, textBody
	return nil
}

// SendWithReplyTo satisfies onboarding.Sender. The Reply-To is discarded here — a
// preview shows the mail, and the header is asserted by the runner's own tests.
func (c *capture) SendWithReplyTo(_ context.Context, _, _, _, subject, htmlBody, textBody string) error {
	c.subject, c.html, c.text = subject, htmlBody, textBody
	return nil
}

// sample runs send against a fresh capture and packages what it produced, together
// with the two pinned-scheme variants the contact sheet toggles between.
func sample(name, title string, send func(*capture) error) (Sample, error) {
	c := &capture{}
	if err := send(c); err != nil {
		return Sample{}, fmt.Errorf("rendering %s: %w", name, err)
	}
	light, err := mailtpl.PinScheme(c.html, mailtpl.SchemeLight)
	if err != nil {
		return Sample{}, fmt.Errorf("pinning %s to light: %w", name, err)
	}
	dark, err := mailtpl.PinScheme(c.html, mailtpl.SchemeDark)
	if err != nil {
		return Sample{}, fmt.Errorf("pinning %s to dark: %w", name, err)
	}
	return Sample{
		Name: name, Title: title, Subject: c.subject,
		HTML: c.html, LightHTML: light, DarkHTML: dark, Text: c.text,
	}, nil
}

// The signup sequence. Its Sender takes a Reply-To, which capture also satisfies —
// these are the only mails that ask for an answer.
func onboardingSample(name, title string, step onboarding.Step, baseURL string) (Sample, error) {
	return sample(name, title, func(c *capture) error {
		return onboarding.NewMailer(c, "notifications@freehire.me", "ilya@freehire.me", baseURL).
			Send(context.Background(), step, "someone@example.com")
	})
}

func welcomeSample(baseURL string) (Sample, error) {
	return onboardingSample("onboarding-welcome", "Onboarding / 1 · Welcome", onboarding.StepWelcome, baseURL)
}

func noAlertSample(baseURL string) (Sample, error) {
	return onboardingSample("onboarding-no-alert", "Onboarding / 2 · No alert yet", onboarding.StepNoAlert, baseURL)
}

func openSourceSample(baseURL string) (Sample, error) {
	return onboardingSample("onboarding-open-source", "Onboarding / 3 · Open source", onboarding.StepOpenSource, baseURL)
}

func verificationSample(baseURL string) (Sample, error) {
	return sample("verify-email", "Account / Verify email", func(c *capture) error {
		return emailnotify.NewAuthMailer(c, "hi@freehire.me", baseURL).
			SendVerificationCode(context.Background(), "someone@example.com", "418205")
	})
}

func passwordResetSample(baseURL string) (Sample, error) {
	return sample("password-reset", "Account / Password reset", func(c *capture) error {
		return emailnotify.NewAuthMailer(c, "hi@freehire.me", baseURL).
			SendPasswordResetCode(context.Background(), "someone@example.com", "730914")
	})
}

// digestSample deliberately mixes jobs with and without a salary, and sets Total
// above the number of jobs listed, so the preview shows both the sparse row and the
// overflow button rather than only the tidy case.
func digestSample(baseURL string) (Sample, error) {
	return sample("subscription-digest", "Alerts / Subscription digest", func(c *capture) error {
		d := notify.Digest{
			SavedSearchName: "Senior Go, remote in Europe",
			Total:           14,
			Jobs: []notify.DigestJob{
				{Title: "Senior Backend Engineer (Go)", Company: "Fingerprint", Slug: "senior-backend-engineer-go-fingerprint",
					SalaryMin: 120000, SalaryMax: 160000, SalaryCurrency: "EUR", SalaryPeriod: "year"},
				{Title: "Staff Engineer, Platform", Company: "Speechify", Slug: "staff-engineer-platform-speechify"},
				{Title: "Go Developer — Payments", Company: "Avenga", Slug: "go-developer-payments-avenga",
					SalaryMin: 90000, SalaryMax: 110000, SalaryCurrency: "USD", SalaryPeriod: "year"},
			},
		}
		return emailnotify.NewNotifier(c, "alerts@freehire.me", baseURL).
			Send(context.Background(), notify.ChannelEmail, "someone@example.com", d)
	})
}

func savedJobReminderSample(baseURL string) (Sample, error) {
	return sample("saved-job-reminder", "Tracking / Saved-job reminder", func(c *capture) error {
		return reminder.NewEmailNotifier(c, "alerts@freehire.me", baseURL).
			Send(context.Background(), notify.ChannelEmail, "someone@example.com", reminder.ReminderMessage{
				JobTitle: "Senior Backend Engineer (Go)",
				Company:  "Fingerprint",
				Slug:     "senior-backend-engineer-go-fingerprint",
			})
	})
}

func followUpNudgeSample(baseURL string) (Sample, error) {
	return sample("nudge-follow-up", "Tracking / Nudge: follow up", func(c *capture) error {
		return nudge.NewEmailNotifier(c, "alerts@freehire.me", baseURL).
			Send(context.Background(), notify.ChannelEmail, "someone@example.com", nudge.Message{
				Kind:       nudge.KindFollowUp,
				JobTitle:   "Staff Engineer, Platform",
				Company:    "Speechify",
				Slug:       "staff-engineer-platform-speechify",
				DaysSilent: 12,
			})
	})
}

func jobClosedNudgeSample(baseURL string) (Sample, error) {
	return sample("nudge-job-closed", "Tracking / Nudge: job closed", func(c *capture) error {
		return nudge.NewEmailNotifier(c, "alerts@freehire.me", baseURL).
			Send(context.Background(), notify.ChannelEmail, "someone@example.com", nudge.Message{
				Kind:     nudge.KindJobClosed,
				JobTitle: "Go Developer — Payments",
				Company:  "Avenga",
				Slug:     "go-developer-payments-avenga",
			})
	})
}

func referralRequestSample(baseURL string) (Sample, error) {
	return sample("referral-request", "Referrals / New request", func(c *capture) error {
		return referral.NewChannelPinger(c, "hi@freehire.me", nil, baseURL).
			PingReferrer(context.Background(),
				referral.Recipient{UserID: 1, Email: "someone@example.com"},
				baseURL+"/my/referrals/inbox")
	})
}

// reportRemovedSample carries a moderator note and a quoted report, the fullest
// version of this mail.
func reportRemovedSample(baseURL string) (Sample, error) {
	return sample("report-job-removed", "Moderation / Report: job removed", func(c *capture) error {
		return report.NewMailNotifier(c, "hi@freehire.me", baseURL).
			NotifyDecision(context.Background(), report.Decision{
				Email:     "someone@example.com",
				JobTitle:  "Go Developer — Payments",
				JobSlug:   "go-developer-payments-avenga",
				Details:   "The posting has been reappearing every week for four months and the form 404s.",
				Note:      "Confirmed — the application form is dead and the company stopped hiring in March.",
				Outcome:   report.OutcomeResolved,
				JobClosed: true,
			})
	})
}

func reportDismissedSample(baseURL string) (Sample, error) {
	return sample("report-dismissed", "Moderation / Report: no change", func(c *capture) error {
		return report.NewMailNotifier(c, "hi@freehire.me", baseURL).
			NotifyDecision(context.Background(), report.Decision{
				Email:    "someone@example.com",
				JobTitle: "Staff Engineer, Platform",
				JobSlug:  "staff-engineer-platform-speechify",
				Details:  "Looks like a duplicate of another posting.",
				Note:     "The two postings are for different teams, so both stay up.",
				Outcome:  report.OutcomeDismissed,
			})
	})
}
