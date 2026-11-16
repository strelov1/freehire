package applicationdigest

import (
	"bytes"
	"fmt"
	"html/template"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/application/mailtpl"
	"github.com/strelov1/freehire/internal/application/userjob"
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
	return &Mailer{emailnotify.From("freehire", from), origin, mailtpl.New(origin), emailprefs.NewLinks(secret, origin)}
}

type stageCount struct {
	Stage string
	Count int64
}
type followupEntry struct {
	Followup
	URL, FollowedUp string
}
type content struct {
	Snapshot
	TrackingURL string
	Period      string
	Stages      []stageCount
	Entries     []followupEntry
}

func (m *Mailer) Render(c Candidate, s Snapshot) (emailnotify.Message, error) {
	unsubscribe, err := m.links.For(c.UserID, emailprefs.GroupActivity)
	if err != nil {
		return emailnotify.Message{}, err
	}
	data := content{Snapshot: s, TrackingURL: m.origin + "/my/tracking"}
	for stage, count := range s.StageCounts {
		data.Stages = append(data.Stages, stageCount{userjob.Label(stage), count})
	}
	sort.Slice(data.Stages, func(i, j int) bool { return data.Stages[i].Stage < data.Stages[j].Stage })
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		loc = time.UTC
	}
	if !s.AsOf.IsZero() {
		data.Period = s.AsOf.Add(-7*24*time.Hour).In(loc).Format("02 Jan 2006") + " – " + s.AsOf.In(loc).Format("02 Jan 2006")
	}
	for _, item := range s.Followups {
		item.Stage = userjob.Label(item.Stage)
		e := followupEntry{Followup: item, URL: data.TrackingURL + "/a" + strconv.FormatInt(item.ID, 10)}
		if !item.FollowedUpAt.IsZero() {
			e.FollowedUp = item.FollowedUpAt.In(loc).Format("02 Jan 2006")
		}
		if item.JobSlug != "" {
			e.URL = data.TrackingURL + "/" + item.JobSlug
		}
		data.Entries = append(data.Entries, e)
	}
	var html bytes.Buffer
	if err := bodyTemplate.Execute(&html, data); err != nil {
		return emailnotify.Message{}, err
	}
	var text strings.Builder
	if data.Period != "" {
		fmt.Fprintf(&text, "%s\n", data.Period)
	}
	fmt.Fprintf(&text, "Your weekly applications\n\nLast 7 days: %d applications, %d replies recorded, %d interview events, %d offer events.\n\n", s.AppliedCount, s.RepliesCount, s.InterviewCount, s.OfferCount)
	if len(data.Stages) > 0 {
		text.WriteString("Current stages\n")
	}
	for _, stage := range data.Stages {
		fmt.Fprintf(&text, "%s: %d\n", stage.Stage, stage.Count)
	}
	if len(data.Entries) > 0 {
		text.WriteString("\nApplications to review\n")
	}
	for _, e := range data.Entries {
		fmt.Fprintf(&text, "%s — %s (%s)\nNo reply recorded for %d days.\n", e.RoleTitle, e.CompanySlug, e.Stage, e.DaysSilent)
		if e.FollowedUp != "" {
			fmt.Fprintf(&text, "Followed up: %s\n", e.FollowedUp)
		}
		text.WriteString(e.URL + "\n\n")
	}
	text.WriteString("Review your tracking board and decide whether to follow up.\n" + data.TrackingURL + "\n")
	text.WriteString(emailprefs.TextFooter(unsubscribe))
	return emailnotify.Message{From: m.from, To: c.Email, Subject: "Your weekly applications", NotificationKind: mailmeta.ApplicationDigest, Group: emailprefs.GroupActivity, UnsubscribeURL: unsubscribe, Text: text.String(), HTML: m.layout.Render(mailtpl.Body{Heading: "Your weekly applications", Preheader: "Your application activity over the last 7 days", Content: template.HTML(html.String()), UnsubscribeURL: unsubscribe})}, nil
}

var bodyTemplate = template.Must(template.New("application-digest").Parse(`
{{if .Period}}<p>{{.Period}}</p>{{end}}
<p>Last 7 days: {{.AppliedCount}} applications, {{.RepliesCount}} replies recorded, {{.InterviewCount}} interview events, {{.OfferCount}} offer events.</p>
{{if .Stages}}<h2>Current stages</h2>{{range .Stages}}<p>{{.Stage}}: {{.Count}}</p>{{end}}{{end}}
{{if .Entries}}<h2>Applications to review</h2>{{range .Entries}}<h3><a href="{{.URL}}">{{.RoleTitle}}</a></h3><p>{{.CompanySlug}} · {{.Stage}}</p><p>No reply recorded for {{.DaysSilent}} days.</p>{{if .FollowedUp}}<p>Followed up: {{.FollowedUp}}</p>{{end}}{{end}}{{end}}
<p>Review your tracking board and decide whether to follow up.</p><p><a href="{{.TrackingURL}}">Open tracking board</a></p>`))
