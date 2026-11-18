// Package mailpreview renders every outgoing freehire email against fixed sample
// data, so the design can be reviewed without sending anything.
//
// Email has no dev server (templates run inside cron workers), so rendering here
// gives each mail a file you can open.
//
// The samples feed two consumers: cmd/mail-preview writes them to disk for
// Storybook, and mailpreview_test.go asserts the committed files still match, so a
// template change nobody re-previewed fails the build.
//
// Adding a mail: write a renderer and list it in `renderers`; the contact sheet,
// Storybook gallery and staleness test all read that list, and
// TestEveryRendererIsRegistered fails if a renderer is left out of it.
//
// No test sees a mail with no renderer AT ALL, so "every outgoing mail" is an
// intent, not a checked invariant.
package mailpreview

import (
	"context"
	"fmt"
	"time"

	"github.com/strelov1/freehire/internal/application/mailtpl"
	"github.com/strelov1/freehire/internal/engage/broadcast"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/engage/nudge"
	"github.com/strelov1/freehire/internal/engage/onboarding"
	"github.com/strelov1/freehire/internal/engage/referral"
	"github.com/strelov1/freehire/internal/engage/reminder"
	"github.com/strelov1/freehire/internal/engage/report"
)

// DefaultBaseURL is what the committed previews link to: the preview directory
// itself, so the logo travels with the files and the output renders off disk and in
// Storybook. Real URLs show in each preview's plain-text panel; pass
// -base=https://freehire.me to see them in the markup.
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
	// Text is the plain-text alternative, shown beside the HTML to compare the two.
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
	inboxDigestSample,
	discordInviteSample,
	interviewBuddiesSample,
	welcomeSample,
	advancedSearchSample,
	noAlertSample,
	extensionSample,
	openSourceSample,
	tailorSample,
	buddiesSample,
	verificationSample,
	passwordResetSample,
	emailChangeSample,
	emailChangedSample,
	directApplicationSample,
	applicationStatusSample,
	teamInviteSample,
	digestSample,
	digestBundleSample,
	savedJobReminderSample,
	savedJobReminderBatchSample,
	followUpNudgeSample,
	followUpNudgeBatchSample,
	jobClosedNudgeSample,
	referralRequestSample,
	reportRemovedSample,
	reportDismissedSample,
}

// capture is a Sender that keeps the last message instead of delivering it; it
// satisfies every mail package's sender interface.
//
// It deliberately does NOT call Message.validate: refusing to render a mail with no
// way out would hide the very mistake a reviewer opened the sheet to see.
type capture struct {
	subject, html, text string
}

func (c *capture) Send(_ context.Context, m emailnotify.Message) error {
	c.subject, c.html, c.text = m.Subject, m.HTML, m.Text
	return nil
}

// previewSecret signs the unsubscribe links in a preview. Fixed and not a secret:
// the mails are never sent, and a varying token would fail the staleness test. It is
// long enough to clear emailprefs' minimum secret length.
const previewSecret = "mail-preview-not-a-real-secret-000"

// previewLinks is the link maker every sample passes to its notifier.
func previewLinks(baseURL string) *emailprefs.Links {
	return emailprefs.NewLinks(previewSecret, baseURL)
}

// previewUserID is the account every sample is addressed to. Fixed, like the secret,
// so tokens do not vary.
const previewUserID int64 = 1

// sample runs send against a fresh capture and packages the result with the two
// pinned-scheme variants.
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

// One-off campaigns.
func campaignSample(name, title, campaignName, baseURL string) (Sample, error) {
	return sample(name, title, func(c *capture) error {
		campaign, ok := broadcast.Lookup(campaignName)
		if !ok {
			return fmt.Errorf("campaign %q is not registered", campaignName)
		}
		return broadcast.NewMailer(c, "notifications@freehire.me", "ilya@freehire.me", baseURL, previewLinks(baseURL)).
			Send(context.Background(), campaign, previewUserID, "someone@example.com")
	})
}

func discordInviteSample(baseURL string) (Sample, error) {
	return campaignSample("campaign-discord-invite", "Campaigns / Discord invite",
		"discord-invite", baseURL)
}

func interviewBuddiesSample(baseURL string) (Sample, error) {
	return campaignSample("campaign-interview-buddies", "Campaigns / Interview buddies",
		"interview-buddies", baseURL)
}

// The signup sequence, the only mails that ask for an answer (Reply-To).
func onboardingSample(name, title string, step onboarding.Step, baseURL string) (Sample, error) {
	return sample(name, title, func(c *capture) error {
		return onboarding.NewMailer(c, "notifications@freehire.me", "ilya@freehire.me", baseURL, previewLinks(baseURL)).
			Send(context.Background(), step, previewUserID, "someone@example.com")
	})
}

func welcomeSample(baseURL string) (Sample, error) {
	return onboardingSample("onboarding-welcome", "Onboarding / 1 · Welcome", onboarding.StepWelcome, baseURL)
}

func advancedSearchSample(baseURL string) (Sample, error) {
	return onboardingSample("onboarding-advanced-search", "Onboarding / 2 · Advanced search", onboarding.StepAdvancedSearch, baseURL)
}

func noAlertSample(baseURL string) (Sample, error) {
	return onboardingSample("onboarding-no-alert", "Onboarding / 3 · No alert yet", onboarding.StepNoAlert, baseURL)
}

func extensionSample(baseURL string) (Sample, error) {
	return onboardingSample("onboarding-extension", "Onboarding / 4 · Browser extension", onboarding.StepExtension, baseURL)
}

func openSourceSample(baseURL string) (Sample, error) {
	return onboardingSample("onboarding-open-source", "Onboarding / 5 · Open source", onboarding.StepOpenSource, baseURL)
}

func tailorSample(baseURL string) (Sample, error) {
	return onboardingSample("onboarding-tailor", "Onboarding / 6 · CV tailoring", onboarding.StepTailor, baseURL)
}

func buddiesSample(baseURL string) (Sample, error) {
	return onboardingSample("onboarding-buddies", "Onboarding / 7 · Interview buddies", onboarding.StepBuddies, baseURL)
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

func emailChangeSample(baseURL string) (Sample, error) {
	return sample("email-change", "Account / Change email", func(c *capture) error {
		return emailnotify.NewAuthMailer(c, "hi@freehire.me", baseURL).
			SendEmailChangeCode(context.Background(), "someone.new@example.com", "264083")
	})
}

func emailChangedSample(baseURL string) (Sample, error) {
	return sample("email-changed", "Account / Email changed (old address)", func(c *capture) error {
		return emailnotify.NewAuthMailer(c, "hi@freehire.me", baseURL).
			SendEmailChangedNotice(context.Background(), "someone@example.com")
	})
}

// directApplicationSample shows every part an employer can receive — a yes/no answer, a
// list answer, a free-text one and a note — so the preview carries the whole layout.
func directApplicationSample(baseURL string) (Sample, error) {
	return sample("direct-application", "Employer / New application", func(c *capture) error {
		return emailnotify.NewApplicationMailer(c, "hi@freehire.me", baseURL).
			SendApplication(context.Background(), emailnotify.ApplicationMail{
				To:             "hiring@acme.example",
				CandidateEmail: "someone@example.com",
				CompanyName:    "Acme",
				JobTitle:       "Senior Platform Engineer",
				JobURL:         baseURL + "/jobs/senior-platform-engineer-acme-9f3c",
				Answers: []emailnotify.ApplicationAnswer{
					{Question: "Do you hold an EU work permit?", Answer: "Yes"},
					{Question: "Preferred stack", Answer: "Go"},
					{Question: "When could you start?", Answer: "In a month — my notice period is four weeks."},
				},
				Note:          "I run a Kubernetes platform for 40 teams today and would love to talk about yours.",
				CV:            []byte("%PDF-1.7"),
				CVFilename:    "cv.pdf",
				CVContentType: "application/pdf",
			})
	})
}

// applicationStatusSample is a rejection with the employer's own words — the letter whose
// tone matters most, and the one carrying every part (lead, quoted message, link).
func applicationStatusSample(baseURL string) (Sample, error) {
	return sample("application-status", "Candidate / Employer's answer", func(c *capture) error {
		return emailnotify.NewApplicationMailer(c, "hi@freehire.me", baseURL).
			SendStatusUpdate(context.Background(), emailnotify.ApplicationStatusMail{
				To:          "someone@example.com",
				ReplyTo:     "hiring@acme.example",
				CompanyName: "Acme",
				JobTitle:    "Senior Platform Engineer",
				JobURL:      baseURL + "/jobs/senior-platform-engineer-acme-9f3c",
				Stage:       "rejected",
				Message:     "Thank you for the time you put into this. We chose someone with more on-call experience, and would be glad to hear from you again next year.",
			})
	})
}

// teamInviteSample is the invitation a colleague receives to join a company's team.
func teamInviteSample(baseURL string) (Sample, error) {
	return sample("team-invite", "Employer / Team invitation", func(c *capture) error {
		return emailnotify.NewTeamInviteMailer(c, "hi@freehire.me", baseURL).
			SendTeamInvitation(context.Background(), emailnotify.TeamInvitation{
				To:          "someone@acme.example",
				CompanyName: "Acme",
				InvitedBy:   "hiring@acme.example",
				Link:        baseURL + "/my/employer/join?token=sample",
				ExpiresAt:   time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC),
			})
	})
}

// digestSample deliberately mixes jobs with and without a salary and sets Total above
// the jobs listed, so the preview shows the sparse row and the overflow button.
func digestSample(baseURL string) (Sample, error) {
	return sample("subscription-digest", "Alerts / Subscription digest", func(c *capture) error {
		d := notify.Digest{
			SavedSearchName: "Senior Go, remote in Europe",
			SearchQuery:     "q=go&seniority=senior&remote=europe",
			Total:           14,
			Jobs: []notify.DigestJob{
				{Title: "Senior Backend Engineer (Go)", Company: "Fingerprint", Slug: "senior-backend-engineer-go-fingerprint",
					SalaryMin: 120000, SalaryMax: 160000, SalaryCurrency: "EUR", SalaryPeriod: "year"},
				{Title: "Staff Engineer, Platform", Company: "Speechify", Slug: "staff-engineer-platform-speechify"},
				{Title: "Go Developer — Payments", Company: "Avenga", Slug: "go-developer-payments-avenga",
					SalaryMin: 90000, SalaryMax: 110000, SalaryCurrency: "USD", SalaryPeriod: "year"},
			},
		}
		return emailnotify.NewNotifier(c, "alerts@freehire.me", baseURL, previewLinks(baseURL)).
			Send(context.Background(), notify.ChannelEmail, "someone@example.com", notify.Message{UserID: previewUserID, Digests: []notify.Digest{d}})
	})
}

// digestBundleSample is one person's due digests in one mail: a section per alert,
// one of them overflowing to its saved search.
func digestBundleSample(baseURL string) (Sample, error) {
	return sample("subscription-digest-bundle", "Alerts / Several alerts in one digest", func(c *capture) error {
		m := notify.Message{UserID: previewUserID, Digests: []notify.Digest{
			{
				SavedSearchName: "Senior Go, remote in Europe",
				SearchQuery:     "q=go&seniority=senior&remote=europe",
				Total:           14,
				Jobs: []notify.DigestJob{
					{Title: "Senior Backend Engineer (Go)", Company: "Fingerprint", Slug: "senior-backend-engineer-go-fingerprint",
						SalaryMin: 120000, SalaryMax: 160000, SalaryCurrency: "EUR", SalaryPeriod: "year"},
					{Title: "Staff Engineer, Platform", Company: "Speechify", Slug: "staff-engineer-platform-speechify"},
				},
			},
			{
				SavedSearchName: "My profile",
				Total:           1,
				NotificationID:  42,
				Jobs: []notify.DigestJob{
					{Title: "Frontend Engineer (React)", Company: "Avenga", Slug: "frontend-engineer-react-avenga"},
				},
			},
		}}
		return emailnotify.NewNotifier(c, "alerts@freehire.me", baseURL, previewLinks(baseURL)).
			Send(context.Background(), notify.ChannelEmail, "someone@example.com", m)
	})
}

func savedJobReminderSample(baseURL string) (Sample, error) {
	return sample("saved-job-reminder", "Tracking / Saved-job reminder", func(c *capture) error {
		return reminder.NewEmailNotifier(c, "alerts@freehire.me", baseURL, previewLinks(baseURL)).
			Send(context.Background(), notify.ChannelEmail, "someone@example.com", []reminder.ReminderMessage{{
				UserID:   previewUserID,
				JobTitle: "Senior Backend Engineer (Go)",
				Company:  "Fingerprint",
				Slug:     "senior-backend-engineer-go-fingerprint",
			}})
	})
}

// savedJobReminderBatchSample is the shape a day of saves produces.
func savedJobReminderBatchSample(baseURL string) (Sample, error) {
	return sample("saved-job-reminder-batch", "Tracking / Saved-job reminders (batch)", func(c *capture) error {
		return reminder.NewEmailNotifier(c, "alerts@freehire.me", baseURL, previewLinks(baseURL)).
			Send(context.Background(), notify.ChannelEmail, "someone@example.com", []reminder.ReminderMessage{
				{UserID: previewUserID, JobTitle: "Senior Backend Engineer (Go)", Company: "Fingerprint", Slug: "senior-backend-engineer-go-fingerprint"},
				{UserID: previewUserID, JobTitle: "Staff Platform Engineer", Company: "Vercel", Slug: "staff-platform-engineer-vercel"},
				{UserID: previewUserID, JobTitle: "Site Reliability Engineer", Company: "Datadog", Slug: "site-reliability-engineer-datadog"},
			})
	})
}

func followUpNudgeSample(baseURL string) (Sample, error) {
	return sample("nudge-follow-up", "Tracking / Nudge: follow up", func(c *capture) error {
		return nudge.NewEmailNotifier(c, "alerts@freehire.me", baseURL, previewLinks(baseURL)).
			Send(context.Background(), notify.ChannelEmail, "someone@example.com", nudge.KindFollowUp, []nudge.Message{{
				UserID:     previewUserID,
				Kind:       nudge.KindFollowUp,
				JobTitle:   "Staff Engineer, Platform",
				Company:    "Speechify",
				Slug:       "staff-engineer-platform-speechify",
				DaysSilent: 12,
			}})
	})
}

// followUpNudgeBatchSample is the shape a pass with several quiet applications produces.
func followUpNudgeBatchSample(baseURL string) (Sample, error) {
	return sample("nudge-follow-up-batch", "Tracking / Nudge: follow up (batch)", func(c *capture) error {
		return nudge.NewEmailNotifier(c, "alerts@freehire.me", baseURL, previewLinks(baseURL)).
			Send(context.Background(), notify.ChannelEmail, "someone@example.com", nudge.KindFollowUp, []nudge.Message{
				{UserID: previewUserID, Kind: nudge.KindFollowUp, JobTitle: "Staff Engineer, Platform", Company: "Speechify", Slug: "staff-engineer-platform-speechify", DaysSilent: 12},
				{UserID: previewUserID, Kind: nudge.KindFollowUp, JobTitle: "Backend Engineer", Company: "Monzo", Slug: "backend-engineer-monzo", DaysSilent: 24},
				{UserID: previewUserID, Kind: nudge.KindFollowUp, JobTitle: "Infrastructure Engineer", Company: "Grafana Labs", Slug: "infrastructure-engineer-grafana-labs", DaysSilent: 31},
			})
	})
}

func jobClosedNudgeSample(baseURL string) (Sample, error) {
	return sample("nudge-job-closed", "Tracking / Nudge: job closed", func(c *capture) error {
		return nudge.NewEmailNotifier(c, "alerts@freehire.me", baseURL, previewLinks(baseURL)).
			Send(context.Background(), notify.ChannelEmail, "someone@example.com", nudge.KindJobClosed, []nudge.Message{{
				UserID:   previewUserID,
				Kind:     nudge.KindJobClosed,
				JobTitle: "Go Developer — Payments",
				Company:  "Avenga",
				Slug:     "go-developer-payments-avenga",
			}})
	})
}

func referralRequestSample(baseURL string) (Sample, error) {
	return sample("referral-request", "Referrals / New request", func(c *capture) error {
		return referral.NewChannelPinger(c, "hi@freehire.me", nil, baseURL, previewLinks(baseURL), nil).
			PingReferrer(context.Background(),
				referral.Recipient{UserID: 1, Email: "someone@example.com"},
				baseURL+"/my/referrals/incoming")
	})
}

// reportRemovedSample carries a moderator note and a quoted report, the fullest
// version of this mail.
func reportRemovedSample(baseURL string) (Sample, error) {
	return sample("report-job-removed", "Moderation / Report: job removed", func(c *capture) error {
		return report.NewMailNotifier(c, "hi@freehire.me", baseURL, previewLinks(baseURL), nil).
			NotifyDecision(context.Background(), report.Decision{
				UserID:    previewUserID,
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
		return report.NewMailNotifier(c, "hi@freehire.me", baseURL, previewLinks(baseURL), nil).
			NotifyDecision(context.Background(), report.Decision{
				UserID:   previewUserID,
				Email:    "someone@example.com",
				JobTitle: "Staff Engineer, Platform",
				JobSlug:  "staff-engineer-platform-speechify",
				Details:  "Looks like a duplicate of another posting.",
				Note:     "The two postings are for different teams, so both stay up.",
				Outcome:  report.OutcomeDismissed,
			})
	})
}
