// Package prowelcome sends the one-time, personal welcome email a first-ever transition
// into a paying tier earns — see openspec/changes/welcome-pro-subscribers.
//
// It sits beside internal/engage/onboarding rather than internal/engage/emailnotify on
// purpose: this is a person introducing himself and inviting a reply (Reply-To a human
// inbox, first-person prose), not a machine reporting a fact. cmd/pro-welcome-mail is the
// only caller — see that binary's own doc for why this cannot be a pass inside
// cmd/billing-sync (internal/identity/billing is layer identity, this package is layer
// engage, and identity must not import engage).
package prowelcome

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/application/mailtpl"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

// Sender delivers one rendered message. *emailnotify.Client satisfies it in production;
// tests inject a fake. Same seam onboarding.Sender uses, for the same reason: the
// Reply-To this letter needs is a field on emailnotify.Message, not a second transport
// method.
type Sender interface {
	Send(ctx context.Context, m emailnotify.Message) error
}

// ErrNotPaying is returned by render/Send when asked to welcome a free tier. There is
// nothing to welcome an account to on the plan everyone starts on, and a caller reaching
// this with TierFree has already made the mistake the candidate query is built to avoid.
var ErrNotPaying = errors.New("prowelcome: refusing to welcome a free-tier account")

// senderName is what the recipient's message list shows — see
// onboarding.senderName's own comment: "freehire" would set up a reply to a company no
// one intends to answer.
const senderName = "Ilya from freehire"

// Mailer renders and sends the welcome letter.
type Mailer struct {
	sender  Sender
	from    string
	replyTo string
	baseURL string
	layout  *mailtpl.Layout
	links   *emailprefs.Links
}

// NewMailer builds a Mailer. `from` is the verified sending address; `replyTo` is the
// human inbox that answers — see onboarding.NewMailer's own comment on why a blank value
// is a configuration error the caller must catch rather than something this type papers
// over.
func NewMailer(sender Sender, from, replyTo, baseURL string, links *emailprefs.Links) *Mailer {
	return &Mailer{
		sender:  sender,
		from:    emailnotify.From(senderName, from),
		replyTo: replyTo,
		baseURL: strings.TrimRight(baseURL, "/"),
		layout:  mailtpl.New(baseURL),
		links:   links,
	}
}

// Send delivers the welcome letter for one newly-paying account. Refuses TierFree — see
// ErrNotPaying.
func (m *Mailer) Send(ctx context.Context, userID int64, to string, tier plan.Tier, until time.Time) error {
	unsubscribe, err := m.links.For(userID, emailprefs.GroupNews)
	if err != nil {
		return fmt.Errorf("prowelcome: unsubscribe link for user %d: %w", userID, err)
	}
	mail, err := m.render(tier, until, unsubscribe)
	if err != nil {
		return err
	}
	return m.sender.Send(ctx, emailnotify.Message{
		From: m.from, To: to, ReplyTo: m.replyTo, Subject: mail.subject,
		HTML: mail.html, Text: mail.text,
		Group: emailprefs.GroupNews, UnsubscribeURL: unsubscribe,
	})
}

// rendered is the letter's three parts.
type rendered struct {
	subject, html, text string
}

// content is what the body template renders from.
type content struct {
	TierLabel    string
	UntilLabel   string
	DiscordURL   string
	DiscordIcon  string
	LinkedInURL  string
	LinkedInIcon string
	PortraitURL  string
	// IntegrationsURL is where the reader connects their Discord account to claim the
	// paid-tier channel — see internal/engage/discordlink's own "every paying tier gets
	// the same role" for why this letter never needs to distinguish Pro from Ultra here.
	IntegrationsURL string
}

// tierLabel is the ONLY place a plan.Tier becomes the word this letter uses for it — see
// discordlink's "every paying tier gets the same role" for the sibling reasoning: Pro and
// Ultra get the same letter, differing only in which word appears where.
func tierLabel(t plan.Tier) (string, error) {
	switch t {
	case plan.TierPro:
		return "Pro", nil
	case plan.TierUltra:
		return "Ultra", nil
	default:
		return "", ErrNotPaying
	}
}

func (m *Mailer) render(tier plan.Tier, until time.Time, unsubscribe string) (rendered, error) {
	label, err := tierLabel(tier)
	if err != nil {
		return rendered{}, err
	}

	c := content{
		TierLabel:       label,
		UntilLabel:      until.Format("January 2, 2006"),
		DiscordURL:      mailtpl.DiscordURL,
		DiscordIcon:     m.baseURL + "/email-icon-discord.png",
		LinkedInURL:     mailtpl.LinkedInURL,
		LinkedInIcon:    m.baseURL + "/email-icon-linkedin.png",
		PortraitURL:     m.baseURL + "/ilya.jpg",
		IntegrationsURL: m.baseURL + "/my/integrations?utm_source=email",
	}

	var buf bytes.Buffer
	if err := body.Execute(&buf, c); err != nil {
		return rendered{}, fmt.Errorf("prowelcome: rendering welcome letter: %w", err)
	}

	html := m.layout.Render(mailtpl.Body{
		Preheader:      "Thanks for subscribing — a personal note from the founder.",
		Heading:        "Welcome to " + label,
		Content:        template.HTML(buf.String()), //nolint:gosec // trusted template over package constants; no user data reaches it
		Footer:         "You’re getting this because you subscribed to freehire.",
		UnsubscribeURL: unsubscribe,
	})
	return rendered{
		subject: "Welcome to freehire " + label + "!",
		html:    html,
		text:    text(label, c.UntilLabel, c.IntegrationsURL) + emailprefs.TextFooter(unsubscribe),
	}, nil
}
