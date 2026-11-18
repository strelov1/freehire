package emailnotify

import (
	"bytes"
	"context"
	"html/template"
	"time"

	"github.com/strelov1/freehire/internal/application/mailtpl"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

// TeamInviteMailer sends the invitation to join a company's team on freehire
// (internal/ingest/employer, team.go). Its own type because its recipient has no say in
// receiving it — a colleague typed their address — and it carries a single-use link.
type TeamInviteMailer struct {
	sender Sender
	from   string
	layout *mailtpl.Layout
}

// NewTeamInviteMailer builds a TeamInviteMailer sending from `from` through sender.
func NewTeamInviteMailer(sender Sender, from, baseURL string) *TeamInviteMailer {
	return &TeamInviteMailer{sender: sender, from: From(productName, from), layout: mailtpl.New(baseURL)}
}

// TeamInvitation is one invitation as the colleague receives it. This package cannot import
// internal/ingest/employer (the blocks share a layer); the handler wiring translates.
type TeamInvitation struct {
	To          string
	CompanyName string
	InvitedBy   string // the inviting colleague's work email
	Link        string // the join page with the token
	ExpiresAt   time.Time
}

var teamInviteHTML = template.Must(mailtpl.Partials().New("team-invite").Parse(`
{{template "lead" (printf "%s invited you to hire for %s on freehire." .InvitedBy .CompanyName)}}
{{template "p" "You will see the applications to the company's vacancies, move candidates through its pipeline, and publish vacancies of your own."}}
{{template "button" (mailLink .Link "Join the team")}}
{{template "muted" (printf "The link works once, until %s, and adds you under this address." (.ExpiresAt.Format "2 January 2006"))}}
`))

// SendTeamInvitation delivers the invitation. GroupEssential with no unsubscribe link: it is
// one letter a colleague asked for on the recipient's behalf, which expires on its own, and
// there is no stream of them to opt out of.
func (m *TeamInviteMailer) SendTeamInvitation(ctx context.Context, inv TeamInvitation) error {
	var content bytes.Buffer
	if err := teamInviteHTML.Execute(&content, inv); err != nil {
		return err
	}
	return m.sender.Send(ctx, Message{
		From:    m.from,
		To:      inv.To,
		ReplyTo: inv.InvitedBy,
		Subject: "Join " + inv.CompanyName + " on freehire",
		HTML: m.layout.Render(mailtpl.Body{
			Preheader: inv.InvitedBy + " invited you to hire for " + inv.CompanyName,
			Heading:   "Join " + inv.CompanyName,
			Content:   template.HTML(content.String()),                                                                                                                        //nolint:gosec // rendered by the trusted template above, which escaped every value in context
			Footer:    template.HTML("If you do not know " + template.HTMLEscapeString(inv.InvitedBy) + ", ignore this message — nothing happens unless the link is opened."), //nolint:gosec // the only interpolated value is escaped
		}),
		Text: inv.InvitedBy + " invited you to hire for " + inv.CompanyName + " on freehire.\n\n" +
			"Join the team: " + inv.Link + "\n\nThe link works once, until " + inv.ExpiresAt.Format("2 January 2006") + ".\n",
		Group: emailprefs.GroupEssential,
	})
}
