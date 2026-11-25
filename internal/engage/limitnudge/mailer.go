package limitnudge

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

// Mailer renders and sends the limit-nudge letter.
type Mailer struct {
	*foundermail.Mailer
	baseURL string
	layout  *mailtpl.Layout
}

// NewMailer builds a Mailer. `from` is the verified sending address; `replyTo` is the
// human inbox that answers. A blank replyTo is a configuration error the caller must
// catch.
func NewMailer(sender emailnotify.Sender, from, replyTo, baseURL string, links *emailprefs.Links) *Mailer {
	base := strings.TrimRight(baseURL, "/")
	return &Mailer{
		Mailer:  foundermail.New(sender, from, replyTo, links),
		baseURL: base,
		layout:  mailtpl.New(base),
	}
}

// Send delivers the nudge letter to one free-tier account that hit a plan ceiling.
func (m *Mailer) Send(ctx context.Context, userID int64, to string) error {
	unsubscribe, err := m.Unsubscribe(userID)
	if err != nil {
		return fmt.Errorf("limitnudge: unsubscribe link for user %d: %w", userID, err)
	}
	mail, err := m.render(unsubscribe)
	if err != nil {
		return err
	}
	return m.Mailer.Send(ctx, to, unsubscribe, emailnotify.Message{
		Subject: mail.subject, HTML: mail.html, Text: mail.text,
	})
}

// rendered is the letter's three parts.
type rendered struct {
	subject, html, text string
}

// content is what the body template renders from.
type content struct {
	PricingURL   string
	LinkedInURL  string
	LinkedInIcon string
	PortraitURL  string
}

func (m *Mailer) render(unsubscribe string) (rendered, error) {
	c := content{
		PricingURL:   m.baseURL + "/pricing?utm_source=email",
		LinkedInURL:  mailtpl.LinkedInURL,
		LinkedInIcon: m.baseURL + "/email-icon-linkedin.png",
		PortraitURL:  m.baseURL + "/ilya.jpg",
	}

	var buf bytes.Buffer
	if err := body.Execute(&buf, c); err != nil {
		return rendered{}, fmt.Errorf("limitnudge: rendering nudge letter: %w", err)
	}

	html := m.layout.Render(mailtpl.Body{
		Preheader:      "Pro removes the daily ceiling — a personal note from the founder.",
		Heading:        "Ran into today's limit?",
		Content:        template.HTML(buf.String()), //nolint:gosec // trusted template over package constants; no user data reaches it
		Footer:         "You’re getting this because you signed up for freehire.",
		UnsubscribeURL: unsubscribe,
	})
	return rendered{
		subject: "Ran into today's limit?",
		html:    html,
		text:    text(c.PricingURL) + emailprefs.TextFooter(unsubscribe),
	}, nil
}
