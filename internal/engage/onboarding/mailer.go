// Package onboarding sends the founder's signup sequence: five mails over a new
// account's first days, written in the first person and inviting a reply.
//
// It is deliberately unlike the other mail features in this codebase. A digest or a
// nudge is a machine reporting a fact; these are a person introducing himself, and
// the whole point is the reply that comes back. That shapes two decisions the rest
// of the mail stack does not share:
//
//   - the mails carry a Reply-To pointing at a human inbox, not the send address;
//   - the copy lives here as prose rather than being assembled from data, because
//     there is no data — every recipient gets the same five letters.
//
// The sequence is: welcome (immediately), advanced_search (day 3, everyone),
// no_alert (day 6, only if the account never created an alert), extension (day 8,
// everyone), open_source (day 10, everyone). Runner owns when; this file owns what
// they say.
package onboarding

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/strelov1/freehire/internal/application/mailtpl"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
)

// Sender delivers one rendered message with a Reply-To. It is a narrower contract
// than emailnotify.Sender, which has no Reply-To — the difference is the point of
// this feature, so it gets its own seam rather than widening the shared one.
// *emailnotify.Client satisfies it.
type Sender interface {
	SendWithReplyTo(ctx context.Context, from, replyTo, to, subject, htmlBody, textBody string) error
}

// Step names one mail in the sequence. The values are the ledger's `step` column
// and are checked by a constraint there, so they are not free-form.
type Step string

const (
	StepWelcome        Step = "welcome"
	StepAdvancedSearch Step = "advanced_search"
	StepNoAlert        Step = "no_alert"
	StepExtension      Step = "extension"
	StepOpenSource     Step = "open_source"
)

// Links the sequence points at. They are constants rather than configuration
// because they are part of the copy: the mails talk about *this* repository, and a
// deployment that pointed them elsewhere would be lying. The Discord invite is the
// shell's (mailtpl.DiscordURL) — every mail's footer already carries it, and a
// second copy here is a link that can disagree with itself.
const (
	repoURL = "https://github.com/strelov1/freehire"
	// storeURL is the extension's listing. It is the Chrome Web Store's own id — the
	// one thing in this file that cannot be re-derived if it is wrong, since a
	// mistyped id is a live page for somebody else's extension rather than a 404.
	storeURL    = "https://chromewebstore.google.com/detail/freehire/ijfaechijopdlikalojadpojmpilplnj"
	linkedInURL = "https://www.linkedin.com/in/istrelov/"
)

// Mailer renders and sends the sequence.
type Mailer struct {
	sender  Sender
	from    string
	replyTo string
	baseURL string
	layout  *mailtpl.Layout
}

// NewMailer builds a Mailer. `from` is the verified sending address; `replyTo` is
// the human inbox that answers — without it these mails ask for a reply that would
// land in an unattended mailbox, so a blank value is a configuration error the
// caller is expected to catch, not something this type papers over.
func NewMailer(sender Sender, from, replyTo, baseURL string) *Mailer {
	base := strings.TrimRight(baseURL, "/")
	return &Mailer{
		sender:  sender,
		from:    emailnotify.From(senderName, from),
		replyTo: replyTo,
		baseURL: base,
		layout:  mailtpl.New(base),
	}
}

// senderName is what the recipient's message list shows. These letters are from a
// person and say so in the first line, so the sender reads as one too — "freehire"
// would set up a reply to a company that no one intends to answer.
const senderName = "Ilya from freehire"

// Send delivers one step to one address.
func (m *Mailer) Send(ctx context.Context, step Step, to string) error {
	mail, err := m.render(step)
	if err != nil {
		return err
	}
	return m.sender.SendWithReplyTo(ctx, m.from, m.replyTo, to, mail.subject, mail.html, mail.text)
}

// rendered is one mail's three parts.
type rendered struct {
	subject, html, text string
}

// content is what the body templates render from: the absolute URLs that cannot be
// baked into the prose because they depend on the site origin.
type content struct {
	AlertsURL         string
	AdvancedSearchURL string
	ExtensionURL      string
	StoreURL          string
	GitHubIcon        string
	ChromeIcon        string
	DiscordIcon       string
	LinkedInIcon      string
	PortraitURL       string
	RepoURL           string
	DiscordURL        string
	LinkedInURL       string
}

func (m *Mailer) render(step Step) (rendered, error) {
	spec, ok := specs[step]
	if !ok {
		return rendered{}, fmt.Errorf("onboarding: unknown step %q", step)
	}

	var body bytes.Buffer
	if err := spec.body.Execute(&body, m.content()); err != nil {
		return rendered{}, fmt.Errorf("onboarding: rendering %s: %w", step, err)
	}

	html := m.layout.Render(mailtpl.Body{
		Preheader: spec.preheader,
		Heading:   spec.heading,
		Content:   template.HTML(body.String()), //nolint:gosec // trusted templates over package constants; no user data reaches them
		Footer:    "You’re getting this because you signed up for freehire.",
	})
	return rendered{subject: spec.subject, html: html, text: spec.text(m.baseURL)}, nil
}

func (m *Mailer) content() content {
	return content{
		AlertsURL:         m.baseURL + "/my/notifications?utm_source=email",
		AdvancedSearchURL: m.baseURL + "/features/advanced-search?utm_source=email",
		ExtensionURL:      m.baseURL + "/features/extension?utm_source=email",
		StoreURL:          storeURL,
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
