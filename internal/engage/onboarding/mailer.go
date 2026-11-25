// Package onboarding sends the founder's signup sequence: seven first-person mails over a
// new account's first days, with a Reply-To pointing at a human inbox. The copy lives here
// as prose because every recipient gets the same letters.
// The sequence is: welcome (immediately), advanced_search (day 3, everyone),
// no_alert (day 6, only if the account never created an alert), extension (day 8,
// everyone), open_source (day 10, everyone), tailor (day 12, only accounts not on a
// paid tier), buddies (day 14, only accounts that never joined interview buddies and
// were not sent its broadcast). Runner owns when; this file owns what they say.
package onboarding

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/engage/foundermail"
	"github.com/strelov1/freehire/internal/engage/mailtpl"
)

// Step names one mail in the sequence. The values are the ledger's `step` column
// and are checked by a constraint there, so they are not free-form.
type Step string

const (
	StepWelcome        Step = "welcome"
	StepAdvancedSearch Step = "advanced_search"
	StepNoAlert        Step = "no_alert"
	StepExtension      Step = "extension"
	StepOpenSource     Step = "open_source"
	StepTailor         Step = "tailor"
	StepBuddies        Step = "buddies"
)

// Links the sequence points at. Constants, not configuration: they are part of the copy.
// The Discord invite is the shell's (mailtpl.DiscordURL) so it cannot disagree with the footer's.
const (
	repoURL = "https://github.com/strelov1/freehire"
	// storeURL is the extension's Chrome Web Store listing. A mistyped id is a live page for
	// somebody else's extension, not a 404.
	storeURL    = "https://chromewebstore.google.com/detail/freehire/ijfaechijopdlikalojadpojmpilplnj"
	firefoxURL  = "https://addons.mozilla.org/firefox/addon/freehire/"
	linkedInURL = "https://www.linkedin.com/in/istrelov/"
)

// Mailer renders and sends the sequence.
type Mailer struct {
	*foundermail.Mailer
	baseURL string
	layout  *mailtpl.Layout
}

// NewMailer builds a Mailer. from is the verified sending address; replyTo is the human
// inbox that answers. A blank replyTo is a configuration error the caller must catch.
func NewMailer(sender emailnotify.Sender, from, replyTo, baseURL string, links *emailprefs.Links) *Mailer {
	base := strings.TrimRight(baseURL, "/")
	return &Mailer{
		Mailer:  foundermail.New(sender, from, replyTo, links),
		baseURL: base,
		layout:  mailtpl.New(base),
	}
}

// Send delivers one step to one recipient.
func (m *Mailer) Send(ctx context.Context, step Step, userID int64, to string) error {
	unsubscribe, err := m.Unsubscribe(userID)
	if err != nil {
		return fmt.Errorf("onboarding: unsubscribe link for user %d: %w", userID, err)
	}
	mail, err := m.render(step, unsubscribe)
	if err != nil {
		return err
	}
	return m.Mailer.Send(ctx, to, unsubscribe, emailnotify.Message{
		Subject: mail.subject, HTML: mail.html, Text: mail.text,
	})
}

// rendered is one mail's three parts.
type rendered struct {
	subject, html, text string
}

// content is what the body templates render from: every URL a letter points at.
type content struct {
	AlertsURL         string
	AdvancedSearchURL string
	ExtensionURL      string
	TailorURL         string
	BuddiesURL        string
	BuddiesDemoURL    string
	PricingURL        string
	StoreURL          string
	FirefoxURL        string
	ScreeningURL      string
	GitHubIcon        string
	ChromeIcon        string
	DiscordIcon       string
	LinkedInIcon      string
	PortraitURL       string
	RepoURL           string
	DiscordURL        string
	LinkedInURL       string
}

func (m *Mailer) render(step Step, unsubscribe string) (rendered, error) {
	spec, ok := specs[step]
	if !ok {
		return rendered{}, fmt.Errorf("onboarding: unknown step %q", step)
	}

	var body bytes.Buffer
	if err := spec.body.Execute(&body, m.content()); err != nil {
		return rendered{}, fmt.Errorf("onboarding: rendering %s: %w", step, err)
	}

	html := m.layout.Render(mailtpl.Body{
		Preheader:      spec.preheader,
		Heading:        spec.heading,
		Content:        template.HTML(body.String()), //nolint:gosec // trusted templates over package constants; no user data reaches them
		Footer:         "You’re getting this because you signed up for freehire.",
		UnsubscribeURL: unsubscribe,
	})
	text := spec.text(m.baseURL) + emailprefs.TextFooter(unsubscribe)
	return rendered{subject: spec.subject, html: html, text: text}, nil
}

func (m *Mailer) content() content {
	return content{
		AlertsURL:         m.baseURL + "/my/notifications?utm_source=email",
		AdvancedSearchURL: m.baseURL + "/features/advanced-search?utm_source=email",
		ExtensionURL:      m.baseURL + "/features/extension?utm_source=email",
		TailorURL:         m.baseURL + "/features/tailor?utm_source=email",
		BuddiesURL:        m.baseURL + "/features/interview-buddies?utm_source=email",
		BuddiesDemoURL:    m.baseURL + "/email-buddies-demo.gif",
		PricingURL:        m.baseURL + "/pricing?utm_source=email",
		StoreURL:          storeURL,
		FirefoxURL:        firefoxURL,
		ScreeningURL:      m.baseURL + "/my/profile/screening?utm_source=email",
		GitHubIcon:        m.baseURL + "/email-icon-github.png",
		ChromeIcon:        m.baseURL + "/email-icon-chrome.png",
		DiscordIcon:       m.baseURL + "/email-icon-discord.png",
		LinkedInIcon:      m.baseURL + "/email-icon-linkedin.png",
		PortraitURL:       m.baseURL + "/ilya.jpg",
		RepoURL:           repoURL,
		DiscordURL:        mailtpl.DiscordURL,
		LinkedInURL:       linkedInURL,
	}
}
