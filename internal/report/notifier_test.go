package report_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/strelov1/freehire/internal/report"
)

// fakeSender captures the one message the notifier hands the transport.
type fakeSender struct {
	from, to, subject, html, text string
	called                        bool
	err                           error
}

func (s *fakeSender) Send(_ context.Context, from, to, subject, htmlBody, textBody string) error {
	s.from, s.to, s.subject, s.html, s.text = from, to, subject, htmlBody, textBody
	s.called = true
	return s.err
}

// notice delivers one decision through a MailNotifier and returns what was sent.
func notice(t *testing.T, d report.Decision) *fakeSender {
	t.Helper()
	sender := &fakeSender{}
	n := report.NewMailNotifier(sender, "hi@freehire.me", "https://freehire.me")
	if err := n.NotifyDecision(context.Background(), d); err != nil {
		t.Fatalf("NotifyDecision: %v", err)
	}
	if !sender.called {
		t.Fatal("nothing was sent")
	}
	return sender
}

// reported is a decision on a real-looking report; each test varies the outcome.
func reported() report.Decision {
	return report.Decision{
		Email:    "lina@example.test",
		JobTitle: "Senior Web Designer",
		JobSlug:  "senior-web-designer-incogni-1234",
		Details:  "This job is not Remote, in their original post it says Hybrid",
		Outcome:  report.OutcomeResolved,
	}
}

func TestNotice_ResolvedWithTheJobClosed(t *testing.T) {
	d := reported()
	d.JobClosed = true
	d.Note = "You were right, we took it down"
	got := notice(t, d)

	// From carries a display name over the configured address — see senderFrom.
	if got.to != "lina@example.test" {
		t.Errorf("to = %q, want the reporter's address", got.to)
	}
	if !strings.Contains(got.from, "hi@freehire.me") || !strings.Contains(got.from, "freehire") {
		t.Errorf("from = %q, want the configured address behind a readable name", got.from)
	}
	if !strings.Contains(strings.ToLower(got.subject), "removed") {
		t.Errorf("subject = %q, want it to say the job was removed", got.subject)
	}
	if !strings.Contains(got.html, "You were right, we took it down") {
		t.Error("the moderator's note is missing from the HTML body")
	}
	if !strings.Contains(got.text, "You were right, we took it down") {
		t.Error("the moderator's note is missing from the text body")
	}
}

func TestNotice_ResolvedWithTheJobLeftOpen(t *testing.T) {
	d := reported()
	d.Note = "Fixed — the job is now marked hybrid"
	got := notice(t, d)

	if !strings.Contains(got.subject, "Senior Web Designer") {
		t.Errorf("subject = %q, want the job title in it", got.subject)
	}
	if strings.Contains(strings.ToLower(got.subject), "removed") {
		t.Errorf("subject = %q, must not claim the job was removed", got.subject)
	}
	if !strings.Contains(got.html, "Fixed — the job is now marked hybrid") {
		t.Error("the moderator's note is missing from the body")
	}
	if !strings.Contains(got.html, "senior-web-designer-incogni-1234") {
		t.Error("the body does not link to the reported job")
	}
	if !strings.Contains(got.html, "This job is not Remote") {
		t.Error("the reporter's own words are not quoted back")
	}
}

func TestNotice_Dismissed(t *testing.T) {
	d := reported()
	d.Outcome = report.OutcomeDismissed
	d.Note = "The source really does say remote"
	got := notice(t, d)

	if strings.Contains(strings.ToLower(got.subject), "removed") {
		t.Errorf("subject = %q, a dismissal removed nothing", got.subject)
	}
	if !strings.Contains(got.html, "The source really does say remote") {
		t.Error("the dismissal reason is missing from the body")
	}
}

func TestNotice_EachOutcomeReadsDifferently(t *testing.T) {
	closed, open, dismissed := reported(), reported(), reported()
	closed.JobClosed = true
	dismissed.Outcome = report.OutcomeDismissed

	a, b, c := notice(t, closed).subject, notice(t, open).subject, notice(t, dismissed).subject
	if a == b || b == c || a == c {
		t.Errorf("the three outcomes must be distinguishable: %q / %q / %q", a, b, c)
	}
}

func TestNotice_EscapesModeratorAndReporterText(t *testing.T) {
	d := reported()
	d.Note = `<script>alert("note")</script>`
	d.Details = `<img src=x onerror=alert("details")>`
	d.JobTitle = `Senior <b>Web</b> Designer`
	got := notice(t, d)

	// The tags must arrive as inert text, not as markup: escaping `<` is what disarms
	// them, so assert on the delimiters rather than on words like "onerror" that survive
	// harmlessly inside escaped text.
	//
	// The hostile fragments are matched whole rather than by tag name alone, because the
	// branded shell contributes an <img> of its own for the logo — a bare "<img" probe
	// would now flag our own markup.
	for _, raw := range []string{"<script", "<img src=x", "<b>Web</b>"} {
		if strings.Contains(got.html, raw) {
			t.Errorf("unescaped %q reached the HTML body", raw)
		}
	}
	if !strings.Contains(got.html, "&lt;script") || !strings.Contains(got.html, "&lt;img") {
		t.Errorf("the hostile input should appear escaped, not dropped:\n%s", got.html)
	}
}

func TestNotice_BlankNoteDoesNotProduceAnEmptyQuotation(t *testing.T) {
	for _, outcome := range []string{report.OutcomeResolved, report.OutcomeDismissed} {
		d := reported()
		d.Outcome, d.Note = outcome, "   "
		got := notice(t, d)

		if strings.Contains(got.html, "What we did:") || strings.Contains(got.html, "blockquote") {
			t.Errorf("%s: a blank note still rendered a quotation block:\n%s", outcome, got.html)
		}
		if strings.TrimSpace(got.text) == "" {
			t.Errorf("%s: the text body is empty", outcome)
		}
	}
}

func TestNotice_LongReportIsTruncated(t *testing.T) {
	d := reported()
	d.Details = strings.Repeat("x", 4000)
	got := notice(t, d)

	// Assert on the quotation itself rather than on the size of the whole mail: the
	// branded shell has a fixed cost of its own, so a byte budget for the document
	// measures the layout as much as the truncation it is meant to check.
	// 1000 is comfortably above the package's quotation cap and well below the 4000
	// submitted, so the probe stays valid if the exact cap is retuned.
	if strings.Contains(got.html, strings.Repeat("x", 1000)) {
		t.Error("the full report reached the HTML body; the quotation should be capped")
	}
	if !strings.Contains(got.html, "…") {
		t.Error("a truncated quotation should show that it was cut")
	}
}

func TestNotice_TruncationKeepsValidUTF8(t *testing.T) {
	// A report with no spaces in a multi-byte script — plausible for a Russian or CJK
	// reporter — is where a byte-slice cut lands mid-rune and garbles the quotation.
	d := reported()
	d.Details = strings.Repeat("这个职位不是远程的", 200)
	got := notice(t, d)

	if !utf8.ValidString(got.html) {
		t.Error("the HTML body is not valid UTF-8: the quotation was cut mid-rune")
	}
	if !utf8.ValidString(got.text) {
		t.Error("the text body is not valid UTF-8: the quotation was cut mid-rune")
	}
}

func TestNotice_PropagatesTransportFailure(t *testing.T) {
	sender := &fakeSender{err: errors.New("ses is down")}
	n := report.NewMailNotifier(sender, "hi@freehire.me", "https://freehire.me")
	if err := n.NotifyDecision(context.Background(), reported()); err == nil {
		t.Fatal("a transport failure must reach the caller, which decides what it means")
	}
}
