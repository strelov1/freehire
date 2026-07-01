package telegramnotify

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/strelov1/freehire/internal/notify"
)

// telegramMaxLen is Telegram's sendMessage text limit, measured in UTF-16 code units.
const telegramMaxLen = 4096

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
//
// The body is capped to Telegram's telegramMaxLen: job lines are added until the
// next one (plus the largest possible "+ N more" tail) would overflow, then the
// tail absorbs the remainder. Without the cap a digest of many long-title jobs
// exceeds the limit, Telegram rejects the send deterministically, every retry
// re-fails, and the whole batch is dead-lettered — silently dropping the user's
// notifications.
func (n *Notifier) render(d notify.Digest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🔔 <b>%d</b> new job%s for %q\n\n", d.Total, plural(d.Total), html.EscapeString(d.SavedSearchName))

	// Reserve room for the widest possible tail up front (d.Total is its worst-case
	// count), so appending the actual tail after the loop can never push past the limit.
	tailReserve := utf16Len(moreLine(d.Total))
	used := utf16Len(b.String())
	shown := 0
	for _, j := range d.Jobs {
		line := n.jobLine(j)
		lineLen := utf16Len(line)
		if used+lineLen+tailReserve > telegramMaxLen {
			break
		}
		b.WriteString(line)
		used += lineLen
		shown++
	}
	if more := d.Total - shown; more > 0 {
		b.WriteString(moreLine(more))
	}
	return b.String()
}

// jobLine renders one digest job: a bullet linking to the freehire job page, with
// an optional " — Company" suffix. Title and company are HTML-escaped.
func (n *Notifier) jobLine(j notify.DigestJob) string {
	var b strings.Builder
	fmt.Fprintf(&b, "• <a href=%q>%s</a>", n.jobBaseURL+"/jobs/"+j.Slug, html.EscapeString(j.Title))
	if j.Company != "" {
		fmt.Fprintf(&b, " — %s", html.EscapeString(j.Company))
	}
	b.WriteByte('\n')
	return b.String()
}

// moreLine is the "+ N more" overflow tail, or "" when nothing is omitted.
func moreLine(more int) string {
	if more <= 0 {
		return ""
	}
	return fmt.Sprintf("\n+ %d more", more)
}

// utf16Len counts s in UTF-16 code units — the unit Telegram measures a message
// against — so a supplementary-plane rune (e.g. the 🔔 emoji) correctly counts as two.
func utf16Len(s string) int {
	return len(utf16.Encode([]rune(s)))
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
