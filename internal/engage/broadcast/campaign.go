// Package broadcast sends one-off campaigns: one letter to the whole audience on a date someone picked.
//
// Sibling of internal/engage/onboarding (same shell, human sender, Reply-To) but not its pacing:
// a campaign reaches everyone at once, so it has its own ledger; an accidental repeat hits the entire list.
//
// Adding a campaign is adding an entry to `campaigns`.
package broadcast

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
	// text takes the origin like the HTML side: a campaign linking into freehire cannot spell the URL
	// as a constant. Campaigns that only link outward ignore it.
	text func(base string) string
	// Timezones limits the audience to accounts whose stored timezone is listed; empty sends to everyone.
	// There is no country column, so an account that never reported a timezone is never reached.
	Timezones []string
	// DataPlanOnly limits the audience to accounts holding a live Data plan.
	DataPlanOnly bool
	// Half limits a run to one side of a split test, "a" or "b". Empty sends to everyone.
	// Lookup returns a copy, so setting it for one run leaves the registry alone.
	Half string
}

// Mailer renders and sends campaigns.
type Mailer struct {
	*foundermail.Mailer
	baseURL string
	layout  *mailtpl.Layout
}

// NewMailer builds a Mailer. replyTo is the human inbox that answers; a campaign signed by a person
// and answerable by nobody is worse than none, so the caller is expected to require it.
func NewMailer(sender emailnotify.Sender, from, replyTo, baseURL string, links *emailprefs.Links) *Mailer {
	base := strings.TrimRight(baseURL, "/")
	return &Mailer{
		Mailer:  foundermail.New(sender, from, replyTo, links),
		baseURL: base,
		layout:  mailtpl.New(base),
	}
}

// Send delivers one campaign to one recipient.
func (m *Mailer) Send(ctx context.Context, c Campaign, userID int64, to string) error {
	var body bytes.Buffer
	if err := c.body.Execute(&body, m.assets()); err != nil {
		return fmt.Errorf("broadcast: rendering %s: %w", c.Name, err)
	}
	unsubscribe, err := m.Unsubscribe(userID)
	if err != nil {
		return fmt.Errorf("broadcast: unsubscribe link for user %d: %w", userID, err)
	}
	html := m.layout.Render(mailtpl.Body{
		Preheader:      c.Preheader,
		Heading:        c.Heading,
		Content:        template.HTML(body.String()), //nolint:gosec // trusted templates over package constants; no user data reaches them
		Footer:         "You’re getting this because you signed up for freehire.",
		UnsubscribeURL: unsubscribe,
	})
	return m.Mailer.Send(ctx, to, unsubscribe, emailnotify.Message{
		Subject: c.Subject, HTML: html, Text: c.text(m.baseURL) + emailprefs.TextFooter(unsubscribe),
		Campaign: c.Name, Variant: c.Half,
	})
}

// assets is what the campaign templates render from: image URLs and site links (origin-dependent)
// plus the two outward links. Each link sits beside its own icon so a mark can never point elsewhere.
type assets struct {
	PortraitURL    string
	DiscordIcon    string
	DiscordURL     string
	LinkedInIcon   string
	LinkedInURL    string
	BuddiesURL     string
	BuddiesDemoURL string
	PricingURL     string
	DataDocsURL    string
}

func (m *Mailer) assets() assets {
	return assets{
		PortraitURL:    m.baseURL + "/ilya.jpg",
		DiscordIcon:    m.baseURL + "/email-icon-discord.png",
		DiscordURL:     mailtpl.DiscordURL,
		LinkedInIcon:   m.baseURL + "/email-icon-linkedin.png",
		LinkedInURL:    mailtpl.LinkedInURL,
		BuddiesURL:     m.baseURL + "/features/interview-buddies",
		BuddiesDemoURL: m.baseURL + "/email-buddies-demo.gif",
		PricingURL:     m.baseURL + "/pricing",
		DataDocsURL:    m.baseURL + "/docs/api/data",
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
