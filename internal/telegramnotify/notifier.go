package telegramnotify

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/notify"
)

// Compile-time guarantee that Notifier satisfies the channel abstraction.
var _ notify.Notifier = (*Notifier)(nil)

// Notifier is the Telegram implementation of notify.Notifier: it renders a digest
// to an HTML message and sends it to the recipient chat. Digest links point at the
// freehire job page (jobBaseURL/jobs/<slug>) so notifications keep the user on the
// platform and never expose a source URL that may be login-gated.
type Notifier struct {
	client     *Client
	jobBaseURL string
}

// NewNotifier builds a Notifier sending through client, with digest links rooted
// at jobBaseURL (the frontend origin).
func NewNotifier(client *Client, jobBaseURL string) *Notifier {
	return &Notifier{client: client, jobBaseURL: strings.TrimRight(jobBaseURL, "/")}
}

// Send renders the digest and posts it to the chat encoded in dest. The channel
// argument is ignored — this Notifier only serves the telegram channel, which the
// worker routes to it.
func (n *Notifier) Send(ctx context.Context, _ string, dest string, d notify.Digest) error {
	chatID, err := strconv.ParseInt(dest, 10, 64)
	if err != nil {
		return fmt.Errorf("telegramnotify: invalid chat id %q: %w", dest, err)
	}
	return n.client.SendMessage(ctx, chatID, n.render(d))
}

// render builds the HTML message body. Job titles, company names, and the saved
// search name are HTML-escaped (they are user/source data); the freehire URL is
// our own and safe.
func (n *Notifier) render(d notify.Digest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🔔 <b>%d</b> new job%s for %q\n\n", d.Total, plural(d.Total), html.EscapeString(d.SavedSearchName))
	for _, j := range d.Jobs {
		link := n.jobBaseURL + "/jobs/" + j.Slug
		fmt.Fprintf(&b, "• <a href=%q>%s</a>", link, html.EscapeString(j.Title))
		if j.Company != "" {
			fmt.Fprintf(&b, " — %s", html.EscapeString(j.Company))
		}
		b.WriteByte('\n')
	}
	if more := d.Total - len(d.Jobs); more > 0 {
		fmt.Fprintf(&b, "\n+ %d more", more)
	}
	return b.String()
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
