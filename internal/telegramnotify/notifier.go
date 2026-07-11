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

// render builds the HTML message body. Job titles, company names, the salary
// string, and the saved search name are HTML-escaped (they are user/source data);
// the freehire URL is our own and safe.
//
// The body is capped to Telegram's telegramMaxLen: job cards are added until the
// next one (plus the largest possible "+ N more" tail) would overflow, then the
// tail absorbs the remainder. Without the cap a digest of many jobs exceeds the
// limit, Telegram rejects the send deterministically, every retry re-fails, and
// the whole batch is dead-lettered — silently dropping the user's notifications.
func (n *Notifier) render(d notify.Digest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🔔 <b>%d</b> new job%s for %q\n\n", d.Total, plural(d.Total), html.EscapeString(d.SavedSearchName))

	// Reserve room for the widest possible tail up front (d.Total is its worst-case
	// count), so appending the actual tail after the loop can never push past the limit.
	tailReserve := utf16Len(moreLine(d.Total))
	used := utf16Len(b.String())
	shown := 0
	for _, j := range d.Jobs {
		card := n.jobCard(j)
		cardLen := utf16Len(card)
		if used+cardLen+tailReserve > telegramMaxLen {
			break
		}
		b.WriteString(card)
		used += cardLen
		shown++
	}
	if more := d.Total - shown; more > 0 {
		b.WriteString(moreLine(more))
	}
	return strings.TrimRight(b.String(), "\n")
}

// jobCard renders one digest job as a multi-line card: a title, an optional
// company line, an optional salary line, and an Apply link to the freehire job
// page. Title, company, and salary are HTML-escaped; cards are separated by a
// trailing blank line.
func (n *Notifier) jobCard(j notify.DigestJob) string {
	var b strings.Builder
	fmt.Fprintf(&b, "💼 <b>%s</b>\n", html.EscapeString(j.Title))
	if j.Company != "" {
		fmt.Fprintf(&b, "🏛️ at %s\n", html.EscapeString(j.Company))
	}
	if s := formatSalary(j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod); s != "" {
		fmt.Fprintf(&b, "💰 %s\n", html.EscapeString(s))
	}
	fmt.Fprintf(&b, "✅ <a href=%q>Apply →</a>\n\n", n.applyURL(j))
	return b.String()
}

// applyURL is the on-platform freehire job page for a digest job, tagged with a
// telegram UTM source so the bot's traffic is attributable. Slugs are our own
// normalized values, so the URL needs no escaping.
func (n *Notifier) applyURL(j notify.DigestJob) string {
	return n.jobBaseURL + "/jobs/" + j.Slug + "?utm_source=telegram-bot"
}

// currencySymbols maps the common ISO 4217 codes to a glyph; any other code is
// used as a prefix verbatim (e.g. "PLN 20K").
var currencySymbols = map[string]string{"USD": "$", "EUR": "€", "GBP": "£"}

// formatSalary renders a compensation string like "$130K—$170K / year" from the
// enrichment salary fields, or "" when no figure is known. A zero bound counts as
// absent (matching enrichment's positive-or-nil convention), so a one-sided range
// renders alone. Amounts of 1000+ are abbreviated with a K suffix; smaller figures
// (e.g. hourly rates) are shown in full.
func formatSalary(min, max int, currency, period string) string {
	if min <= 0 && max <= 0 {
		return ""
	}
	sym := currencySymbols[strings.ToUpper(currency)]
	if sym == "" && currency != "" {
		sym = currency + " "
	}
	var amount string
	switch {
	case min > 0 && max > 0 && min != max:
		amount = sym + shortMoney(min) + "—" + sym + shortMoney(max)
	case min > 0:
		amount = sym + shortMoney(min)
	default: // only max is known
		amount = sym + shortMoney(max)
	}
	if period != "" {
		amount += " / " + period
	}
	return amount
}

// shortMoney abbreviates 12000→"12K", 4500→"4.5K", and leaves sub-thousand
// figures (e.g. hourly rates) in full: 950→"950".
func shortMoney(v int) string {
	if v < 1000 {
		return strconv.Itoa(v)
	}
	if v%1000 == 0 {
		return strconv.Itoa(v/1000) + "K"
	}
	return strconv.FormatFloat(float64(v)/1000, 'f', 1, 64) + "K"
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
