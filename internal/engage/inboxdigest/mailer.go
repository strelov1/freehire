package inboxdigest

import (
	"bytes"
	"fmt"
	"html/template"
	"strconv"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/application/mailtpl"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/platform/mailmeta"
)

type Mailer struct {
	from, origin string
	layout       *mailtpl.Layout
	links        *emailprefs.Links
}

func NewMailer(from, origin, secret string) *Mailer {
	origin = strings.TrimRight(origin, "/")
	return &Mailer{from: emailnotify.From("freehire", from), origin: origin, layout: mailtpl.New(origin), links: emailprefs.NewLinks(secret, origin)}
}

type entry struct {
	Item
	URL, Label, Date string
}
type section struct {
	Title   string
	Entries []entry
}
type content struct {
	Sections []section
	InboxURL string
	Overflow int64
}

func (m *Mailer) Render(c Candidate, s Snapshot) (emailnotify.Message, error) {
	unsubscribe, err := m.links.For(c.UserID, emailprefs.GroupActivity)
	if err != nil {
		return emailnotify.Message{}, err
	}
	data := content{Sections: []section{{Title: "Recruiter outreach"}, {Title: "Requests in messages"}, {Title: "Application updates"}}, InboxURL: m.origin + "/my/inbox", Overflow: max(0, s.Total-int64(len(s.Items)))}
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		loc = time.UTC
	}
	for _, item := range s.Items {
		if item.Subject == "" {
			item.Subject = "(No subject)"
		}
		date := ""
		if !item.ReceivedAt.IsZero() {
			date = item.ReceivedAt.In(loc).Format("02 Jan, 15:04 MST")
		}
		group := 2
		switch item.Signal {
		case "screening":
			group = 0
		case "interview_invitation", "assessment", "info_request", "incomplete_application":
			if !item.Replied {
				group = 1
			}
		}
		label := strings.ReplaceAll(item.Signal, "_", " ")
		data.Sections[group].Entries = append(data.Sections[group].Entries, entry{Item: item, URL: data.InboxURL + "?message=" + strconv.FormatInt(item.ID, 10), Label: label, Date: date})
	}
	var html bytes.Buffer
	if err := bodyTemplate.Execute(&html, data); err != nil {
		return emailnotify.Message{}, err
	}
	var text strings.Builder
	text.WriteString("Your hiring inbox\n\nNew messages from your job-search inbox, grouped by conversation.\n\n")
	for _, section := range data.Sections {
		if len(section.Entries) == 0 {
			continue
		}
		text.WriteString(section.Title + "\n")
		for _, e := range section.Entries {
			fmt.Fprintf(&text, "%s — %s (%s)\n", e.Subject, e.Sender, e.Label)
			if e.Date != "" {
				text.WriteString(e.Date + "\n")
			}
			if e.Snippet != "" {
				text.WriteString(e.Snippet + "\n")
			}
			if e.Replied {
				text.WriteString("A later reply from you is visible in this conversation.\n")
			}
			text.WriteString(e.URL + "\n\n")
		}
	}
	if data.Overflow > 0 {
		word := "conversations"
		if data.Overflow == 1 {
			word = "conversation"
		}
		fmt.Fprintf(&text, "%d more %s in your inbox: %s\n", data.Overflow, word, data.InboxURL)
	}
	text.WriteString(emailprefs.TextFooter(unsubscribe))
	return emailnotify.Message{From: m.from, To: c.Email, Subject: "Your daily hiring inbox", NotificationKind: mailmeta.InboxDigest, Group: emailprefs.GroupActivity, UnsubscribeURL: unsubscribe, Text: text.String(), HTML: m.layout.Render(mailtpl.Body{Heading: "Your hiring inbox", Preheader: "New recruiter mail, requests and application updates", Content: template.HTML(html.String()), UnsubscribeURL: unsubscribe})}, nil
}

var bodyTemplate = template.Must(template.New("inbox-digest").Parse(`
<p>New messages from your job-search inbox, grouped by conversation.</p>
{{range .Sections}}{{if .Entries}}<h2>{{.Title}}</h2>{{range .Entries}}
<div style="margin-bottom:24px"><h3><a href="{{.URL}}">{{.Subject}}</a></h3>
<p>{{.Sender}} · {{.Label}}{{if .Date}} · {{.Date}}{{end}}</p>{{if .Snippet}}<p>{{.Snippet}}</p>{{end}}
{{if .Replied}}<p>A later reply from you is visible in this conversation.</p>{{end}}</div>
{{end}}{{end}}{{end}}
{{if .Overflow}}<p><a href="{{.InboxURL}}">{{.Overflow}} more conversation{{if ne .Overflow 1}}s{{end}} in your inbox</a></p>{{end}}
`))
