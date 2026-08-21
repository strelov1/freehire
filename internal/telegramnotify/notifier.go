package telegramnotify

import (
	"context"
	"errors"
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
// A 403 is translated to notify.ErrRecipientGone, the engine-side vocabulary for
// "this recipient is permanently closed to us" — the engine unlinks the chat and
// soft-skips instead of counting a delivery failure it would retry to no purpose.
func (n *Notifier) Send(ctx context.Context, _ string, dest string, d notify.Digest) error {
	chatID, err := strconv.ParseInt(dest, 10, 64)
	if err != nil {
		return fmt.Errorf("telegramnotify: invalid chat id %q: %w", dest, err)
	}
	err = n.client.SendMessage(ctx, chatID, n.render(d))
	if errors.Is(err, ErrChatUnreachable) {
		return fmt.Errorf("%w: %w", notify.ErrRecipientGone, err)
	}
	return err
}

// render builds the HTML message body. Job titles, company names, the salary
// string, and the saved search name are HTML-escaped (they are user/source data);
// the freehire URL is our own and safe.
//
// The listing is bounded twice. notify.ListLimit is the product bound, shared with
// the email channel so the two never disagree about a digest's shape. On top of it
// the body is capped to Telegram's telegramMaxLen: job lines are added until the
// next one (plus the largest possible "+ N more" tail) would overflow, then the
// tail absorbs the remainder. Without the length cap a digest of many long-title
// jobs exceeds the limit, Telegram rejects the send deterministically, every retry
// re-fails, and the whole batch is dead-lettered — silently dropping the user's
// notifications.
func (n *Notifier) render(d notify.Digest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🔔 <b>%d</b> new job%s for %q\n\n", d.Total, notify.Plural(d.Total), html.EscapeString(d.SavedSearchName))

	// Reserve room for the widest possible tail up front (d.Total is its worst-case
	// count), so appending the actual tail after the loop can never push past the limit.
	viewAll := n.viewAllURL(d)
	tailReserve := utf16Len(moreLine(d.Total, viewAll))
	used := utf16Len(b.String())
	shown := 0
	for _, j := range d.Listed() {
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
		b.WriteString(moreLine(more, viewAll))
	}
	return b.String()
}

// jobLine renders one digest job on a single line: a bullet linking to the
// freehire job page, an optional " — Company" suffix, and an optional " · salary"
// suffix. Title, company, and salary are HTML-escaped.
func (n *Notifier) jobLine(j notify.DigestJob) string {
	var b strings.Builder
	fmt.Fprintf(&b, "• <a href=%q>%s</a>", n.applyURL(j), html.EscapeString(j.Title))
	if j.Company != "" {
		fmt.Fprintf(&b, " — %s", html.EscapeString(j.Company))
	}
	if s := j.SalaryString(); s != "" {
		fmt.Fprintf(&b, " · %s", html.EscapeString(s))
	}
	b.WriteByte('\n')
	return b.String()
}

// applyURL is the on-platform freehire job page for a digest job, tagged with a
// telegram UTM source so the bot's traffic is attributable. Slugs are our own
// normalized values, so the URL needs no escaping.
func (n *Notifier) applyURL(j notify.DigestJob) string {
	return n.jobBaseURL + "/jobs/" + j.Slug + "?utm_source=telegram-bot"
}

// viewAllURL is where the "+ N more" tail leads: the digest's own in-app
// notification, whose page lists every job this digest matched. A digest whose
// recording failed carries no id and falls back to the notification section.
func (n *Notifier) viewAllURL(d notify.Digest) string {
	if d.NotificationID == 0 {
		return n.jobBaseURL + "/my/notifications"
	}
	return n.jobBaseURL + "/my/notifications/" + strconv.FormatInt(d.NotificationID, 10) + "/jobs?utm_source=telegram-bot"
}

// moreLine is the "+ N more" overflow tail linking to where the omitted jobs are,
// or "" when nothing is omitted. The URL is our own, so it needs no escaping.
func moreLine(more int, viewAllURL string) string {
	if more <= 0 {
		return ""
	}
	return fmt.Sprintf("\n<a href=%q>+ %d more</a>", viewAllURL, more)
}

// utf16Len counts s in UTF-16 code units — the unit Telegram measures a message
// against — so a supplementary-plane rune (e.g. the 🔔 emoji) correctly counts as two.
func utf16Len(s string) int {
	return len(utf16.Encode([]rune(s)))
}
