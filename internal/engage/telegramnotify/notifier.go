package telegramnotify

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/strelov1/freehire/internal/engage/notify"
)

// MaxMessageLen is Telegram's sendMessage text limit, measured in UTF-16 code
// units. Exported because every engine that can build a multi-job message needs
// the same number: a body over the limit is rejected deterministically, so each
// retry re-fails and the batch is dead-lettered.
const MaxMessageLen = 4096

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
func (n *Notifier) Send(ctx context.Context, _ string, dest string, m notify.Message) error {
	return n.client.SendToChat(ctx, dest, n.render(m))
}

// SendToChat posts html to the chat encoded in dest. A 403 is translated to
// notify.ErrRecipientGone, the engine-side vocabulary for "this recipient is
// permanently closed to us" — the engine unlinks the chat and soft-skips instead
// of counting a delivery failure it would retry to no purpose. Shared by the
// digest, reminder and nudge Telegram notifiers.
func (c *Client) SendToChat(ctx context.Context, dest, html string) error {
	chatID, err := strconv.ParseInt(dest, 10, 64)
	if err != nil {
		return fmt.Errorf("telegramnotify: invalid chat id %q: %w", dest, err)
	}
	err = c.SendMessage(ctx, chatID, html)
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
// the body is capped to Telegram's MaxMessageLen: job lines are added until the
// next one (plus the largest possible "+ N more" tail) would overflow, then the
// tail absorbs the remainder. Without the length cap a digest of many long-title
// jobs exceeds the limit, Telegram rejects the send deterministically, every retry
// re-fails, and the whole batch is dead-lettered — silently dropping the user's
// notifications.
//
// A message of several digests is one Telegram message with a section per digest.
// Every section's heading and widest tail are reserved before any job is listed, so
// the last section is never squeezed out by the first one's jobs.
func (n *Notifier) render(m notify.Message) string {
	var b strings.Builder
	// Not notify.JobCount: the bold wraps the numeral alone, so the markup sits
	// inside the phrase that helper returns whole.
	if len(m.Digests) == 1 {
		d := m.Digests[0]
		fmt.Fprintf(&b, "🔔 <b>%d</b> new job%s for %q\n\n", d.Total, notify.Plural(d.Total), html.EscapeString(d.SavedSearchName))
	} else {
		total := m.Total()
		fmt.Fprintf(&b, "🔔 <b>%d</b> new job%s from %d job alerts\n", total, notify.Plural(total), len(m.Digests))
	}

	// A search link is the user's own query and may run long; shared between
	// sections, its allowance shrinks so the reserve below stays bounded.
	searchTailLen := maxSearchTailLen / len(m.Digests)
	type part struct {
		heading, viewAll string
		d                notify.Digest
	}
	parts := make([]part, len(m.Digests))
	// Reserve room for every widest possible tail up front (d.Total is its worst-case
	// count), so appending the actual tails can never push past the limit.
	reserve := 0
	for i, d := range m.Digests {
		p := part{d: d, viewAll: n.viewAllURL(d, searchTailLen)}
		if len(m.Digests) > 1 {
			p.heading = fmt.Sprintf("\n<b>%s</b> · %s\n", html.EscapeString(d.SavedSearchName), notify.JobCount(d.Total))
		}
		reserve += UTF16Len(p.heading) + UTF16Len(moreLine(d.Total, p.viewAll))
		parts[i] = p
	}
	used := UTF16Len(b.String())
	for _, p := range parts {
		b.WriteString(p.heading)
		reserve -= UTF16Len(p.heading) + UTF16Len(moreLine(p.d.Total, p.viewAll))
		used += UTF16Len(p.heading)
		shown := 0
		for _, j := range p.d.Listed() {
			line := n.jobLine(j)
			lineLen := UTF16Len(line)
			if used+lineLen+UTF16Len(moreLine(p.d.Total, p.viewAll))+reserve > MaxMessageLen {
				break
			}
			b.WriteString(line)
			used += lineLen
			shown++
		}
		if more := p.d.Total - shown; more > 0 {
			tail := moreLine(more, p.viewAll)
			b.WriteString(tail)
			used += UTF16Len(tail)
		}
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
//
// A digest that counts more than it carries (notify.Digest.Overflowed) links to
// the saved search's live results instead, since its own page lists only what it
// carries. The query is the user's own string and may run to thousands of
// characters, while the tail is reserved out of MaxMessageLen before any job is
// listed — so a query longer than maxSearchTailLen falls back to the saved
// searches page, one click from the same results.
func (n *Notifier) viewAllURL(d notify.Digest, searchTailLen int) string {
	if d.Overflowed() {
		if u := n.jobBaseURL + d.SearchPath("telegram-bot"); UTF16Len(u) <= searchTailLen {
			return u
		}
		return n.jobBaseURL + "/my/notifications/searches"
	}
	if d.NotificationID == 0 {
		return n.jobBaseURL + "/my/notifications"
	}
	return n.jobBaseURL + "/my/notifications/" + strconv.FormatInt(d.NotificationID, 10) + "/jobs?utm_source=telegram-bot"
}

// maxSearchTailLen bounds the saved-search URL a tail may carry, leaving most of
// MaxMessageLen for the jobs themselves.
const maxSearchTailLen = 1024

// moreLine is the "+ N more" overflow tail linking to where the omitted jobs are,
// or "" when nothing is omitted. The URL is escaped because a saved search's
// query, which an overflowed digest links to, is the user's own string.
func moreLine(more int, viewAllURL string) string {
	if more <= 0 {
		return ""
	}
	return fmt.Sprintf("\n<a href=\"%s\">+ %d more</a>", html.EscapeString(viewAllURL), more)
}

// UTF16Len counts s in UTF-16 code units — the unit Telegram measures a message
// against — so a supplementary-plane rune (e.g. the 🔔 emoji) correctly counts as
// two. Exported alongside MaxMessageLen: a caller given the limit and not the way
// to measure against it would reach for len(), which is bytes.
func UTF16Len(s string) int {
	return len(utf16.Encode([]rune(s)))
}
