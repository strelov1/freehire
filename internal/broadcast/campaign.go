// Package broadcast sends one-off campaigns: a single letter to the whole audience,
// on a date someone picked.
//
// It is the sibling of internal/onboarding and shares its shape — same shell, same
// human sender, same Reply-To — but not its pacing. A drip decides *when* a person
// is ready for the next letter; a campaign has one moment and everyone gets it at
// once. That difference is the reason for a separate ledger: an accidental repeat
// hits the entire list rather than one account.
//
// Adding a campaign is adding an entry to `campaigns`. Nothing else changes.
package broadcast

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/strelov1/freehire/internal/emailnotify"
	"github.com/strelov1/freehire/internal/mailtpl"
)

// Sender delivers one rendered message with a Reply-To, as internal/onboarding does:
// a campaign is signed by a person and invites an answer.
type Sender interface {
	SendWithReplyTo(ctx context.Context, from, replyTo, to, subject, htmlBody, textBody string) error
}

// senderName is the display name in the recipient's message list.
const senderName = "Ilya from freehire"

// Campaign is one letter: what the inbox shows, what the card says, and the
// plain-text alternative.
type Campaign struct {
	// Name is the ledger key. It is what makes a send idempotent, so renaming a
	// campaign that has already gone out would mail everyone a second time.
	Name      string
	Subject   string
	Preheader string
	Heading   string
	body      *template.Template
	text      string
}

// Mailer renders and sends campaigns.
type Mailer struct {
	sender  Sender
	from    string
	replyTo string
	baseURL string
	layout  *mailtpl.Layout
}

// NewMailer builds a Mailer. replyTo is the human inbox that answers; as with the
// onboarding sequence, a campaign signed by a person and answerable by nobody is
// worse than no campaign, so the caller is expected to require it.
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

// Send delivers one campaign to one address.
func (m *Mailer) Send(ctx context.Context, c Campaign, to string) error {
	var body bytes.Buffer
	if err := c.body.Execute(&body, m.assets()); err != nil {
		return fmt.Errorf("broadcast: rendering %s: %w", c.Name, err)
	}
	html := m.layout.Render(mailtpl.Body{
		Preheader: c.Preheader,
		Heading:   c.Heading,
		Content:   template.HTML(body.String()), //nolint:gosec // trusted templates over package constants; no user data reaches them
		Footer:    "You’re getting this because you signed up for freehire.",
	})
	return m.sender.SendWithReplyTo(ctx, m.from, m.replyTo, to, c.Subject, html, c.text)
}

// assets are the absolute image URLs the templates need; they depend on the origin
// and so cannot be baked into the copy.
type assets struct {
	PortraitURL     string
	ProductHuntIcon string
}

func (m *Mailer) assets() assets {
	return assets{
		PortraitURL:     m.baseURL + "/ilya.jpg",
		ProductHuntIcon: m.baseURL + "/email-icon-producthunt.png",
	}
}

// Lookup returns the campaign registered under name.
func Lookup(name string) (Campaign, bool) {
	c, ok := campaigns[name]
	return c, ok
}

// Names lists every registered campaign, for the worker's usage message.
func Names() []string {
	out := make([]string, 0, len(campaigns))
	for name := range campaigns {
		out = append(out, name)
	}
	return out
}
