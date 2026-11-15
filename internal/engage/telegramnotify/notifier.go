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

// MaxMessageLen is Telegram's sendMessage text limit, in UTF-16 code units. Exported for every
// engine that builds a multi-job message: an over-limit body is rejected deterministically, so
// each retry re-fails and the batch is dead-lettered.
const MaxMessageLen = 4096

// Compile-time guarantee that Notifier satisfies the channel abstraction.
var _ notify.Notifier = (*Notifier)(nil)

// Notifier is the Telegram implementation of notify.Notifier: it renders a digest to an HTML
// message and sends it to the recipient chat. Links point at the freehire job page, never a
// source URL that may be login-gated.
type Notifier struct {
	client     *Client
	jobBaseURL string
}

// NewNotifier builds a Notifier sending through client, with digest links rooted
// at jobBaseURL (the frontend origin).
func NewNotifier(client *Client, jobBaseURL string) *Notifier {
	return &Notifier{client: client, jobBaseURL: strings.TrimRight(jobBaseURL, "/")}
}

// Send renders the digest and posts it to the chat encoded in dest. The channel argument is
// ignored: this Notifier only serves telegram. A 403 becomes notify.ErrRecipientGone (see
// SendToChat).
func (n *Notifier) Send(ctx context.Context, _ string, dest string, m notify.Message) error {
	return n.client.SendToChat(ctx, dest, n.render(m))
}

// SendToChat posts html to the chat encoded in dest. A 403 becomes notify.ErrRecipientGone, so
// the engine unlinks the chat and soft-skips instead of retrying. Shared by the digest,
// reminder and nudge Telegram notifiers.
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

// render builds the HTML message body. Titles, company names, salary and the saved search name
// are HTML-escaped (user/source data); the freehire URL is our own.
//
// The listing is bounded twice: notify.ListLimit (shared with email) and Telegram's
// MaxMessageLen. Job lines are added until the next one plus the largest possible "+ N more"
// tail would overflow, then the tail absorbs the remainder; otherwise an over-limit send fails
// on every retry and dead-letters the batch.
//
// Several digests make one message with a section each. Every section's heading and widest
// tail are reserved before any job is listed, so the last section is never squeezed out.
func (n *Notifier) render(m notify.Message) string {
	var b strings.Builder
	// Not notify.JobCount: the bold wraps the numeral alone.
	if len(m.Digests) == 1 {
		d := m.Digests[0]
		fmt.Fprintf(&b, "🔔 <b>%d</b> new job%s for %q\n\n", d.Total, notify.Plural(d.Total), html.EscapeString(d.SavedSearchName))
	} else {
		total := m.Total()
		fmt.Fprintf(&b, "🔔 <b>%d</b> new job%s from %d job alerts\n", total, notify.Plural(total), len(m.Digests))
	}

	// A search link is the user's own query and may run long; its allowance shrinks as sections
	// share the message.
	searchTailLen := maxSearchTailLen / len(m.Digests)
	type part struct {
		heading, viewAll string
		d                notify.Digest
	}
	parts := make([]part, 0, len(m.Digests))
	// Reserve room for every widest possible tail up front (d.Total is the worst case), so the
	// actual tails cannot overflow.
	//
	// Headings and tails are mandatory, so many alerts alone could overflow the limit. Sections
	// that do not fit fold into one closing line, reserved first so it always fits.
	folded := foldedLine(m.Total(), len(m.Digests), n.jobBaseURL)
	reserve := 0
	used := UTF16Len(b.String())
	for i, d := range m.Digests {
		p := part{d: d, viewAll: n.viewAllURL(d, searchTailLen)}
		if len(m.Digests) > 1 {
			p.heading = fmt.Sprintf("\n<b>%s</b> · %s\n", html.EscapeString(d.SavedSearchName), notify.JobCount(d.Total))
		}
		cost := UTF16Len(p.heading) + UTF16Len(moreLine(d.Total, p.viewAll))
		if i < len(m.Digests)-1 && used+reserve+cost+UTF16Len(folded) > MaxMessageLen ||
			i == len(m.Digests)-1 && used+reserve+cost > MaxMessageLen {
			break
		}
		reserve += cost
		parts = append(parts, p)
	}
	var foldedJobs int
	for _, d := range m.Digests[len(parts):] {
		foldedJobs += d.Total
	}
	if rest := len(m.Digests) - len(parts); rest > 0 {
		folded = foldedLine(foldedJobs, rest, n.jobBaseURL)
		reserve += UTF16Len(folded)
	} else {
		folded = ""
	}
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
	b.WriteString(folded)
	return b.String()
}

// foldedLine is the closing line standing for the alerts a message had no room to list,
// linking to the notification centre.
func foldedLine(jobs, alerts int, baseURL string) string {
	return fmt.Sprintf("\n\n<a href=\"%s/my/notifications\">+ %s in %d more job alert%s</a>", baseURL, notify.JobCount(jobs), alerts, notify.Plural(alerts))
}

// jobLine renders one digest job on a single line: a linked title, an optional company and an
// optional salary, all HTML-escaped.
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

// applyURL is the freehire job page for a digest job, tagged with a telegram UTM source. Slugs
// are our own normalized values, so no escaping.
func (n *Notifier) applyURL(j notify.DigestJob) string {
	return n.jobBaseURL + "/jobs/" + j.Slug + "?utm_source=telegram-bot"
}

// viewAllURL is where the "+ N more" tail leads: the digest's own in-app notification. A digest
// whose recording failed carries no id and falls back to the notification section.
//
// A digest that counts more than it carries (notify.Digest.Overflowed) links to the saved
// search's live results instead. The query is user input and the tail is reserved out of
// MaxMessageLen up front, so one longer than maxSearchTailLen falls back to the saved searches
// page.
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

// moreLine is the "+ N more" overflow tail, or "" when nothing is omitted. The URL is escaped
// because it may carry the user's own query.
func moreLine(more int, viewAllURL string) string {
	if more <= 0 {
		return ""
	}
	return fmt.Sprintf("\n<a href=\"%s\">+ %d more</a>", html.EscapeString(viewAllURL), more)
}

// UTF16Len counts s in UTF-16 code units, the unit Telegram measures against, so a
// supplementary-plane rune (e.g. the 🔔 emoji) counts as two. Exported alongside MaxMessageLen
// so callers do not use len(), which is bytes.
func UTF16Len(s string) int {
	return len(utf16.Encode([]rune(s)))
}
