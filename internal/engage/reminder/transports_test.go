package reminder

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/engage/telegramnotify"
)

// captureSender records the last email a EmailNotifier sent.
type captureSender struct {
	from, to, subject, html, text string
}

func (s *captureSender) Send(_ context.Context, from, to, subject, html, text string) error {
	s.from, s.to, s.subject, s.html, s.text = from, to, subject, html, text
	return nil
}

func TestEmailNotifier_RendersSubjectAndOnPlatformLink(t *testing.T) {
	sender := &captureSender{}
	n := NewEmailNotifier(sender, "jobs@freehire.me", "https://freehire.me/")
	msg := ReminderMessage{JobTitle: "Go Dev", Company: "Acme", Slug: "go-dev-acme", URL: "https://ats/x"}

	if err := n.Send(context.Background(), "email", "u@x.com", []ReminderMessage{msg}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	// From carries a display name over the configured address, so a message list
	// shows "freehire" rather than the address's local part.
	if sender.to != "u@x.com" {
		t.Errorf("to = %q, want the recipient", sender.to)
	}
	if !strings.Contains(sender.from, "jobs@freehire.me") || !strings.Contains(sender.from, "freehire") {
		t.Errorf("from = %q, want the configured address behind a readable name", sender.from)
	}
	if !strings.Contains(sender.subject, "Go Dev") || !strings.Contains(sender.subject, "Acme") {
		t.Errorf("subject = %q, want job + company", sender.subject)
	}
	if !strings.Contains(sender.html, "https://freehire.me/jobs/go-dev-acme") {
		t.Errorf("html link must point on-platform, got %q", sender.html)
	}
	if strings.Contains(sender.html, "https://ats/x") {
		t.Errorf("must not leak the source URL, got %q", sender.html)
	}
}

func TestEmailNotifier_EscapesUserData(t *testing.T) {
	sender := &captureSender{}
	n := NewEmailNotifier(sender, "jobs@freehire.me", "https://freehire.me")
	msg := ReminderMessage{JobTitle: "<script>x</script>", Company: "Acme", Slug: "s", URL: "u"}

	if err := n.Send(context.Background(), "email", "u@x.com", []ReminderMessage{msg}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if strings.Contains(sender.html, "<script>") {
		t.Errorf("title must be HTML-escaped in the body, got %q", sender.html)
	}
}

func TestTelegramNotifier_RendersOnPlatformLink(t *testing.T) {
	n := NewTelegramNotifier(nil, "https://freehire.me/")
	got := n.render([]ReminderMessage{{JobTitle: "Go Dev", Company: "Acme", Slug: "go-dev-acme"}})
	if !strings.Contains(got, "https://freehire.me/jobs/go-dev-acme") {
		t.Errorf("telegram render missing on-platform link: %q", got)
	}
	if !strings.Contains(got, "Go Dev") || !strings.Contains(got, "Acme") {
		t.Errorf("telegram render missing job/company: %q", got)
	}
}

func TestTelegramNotifier_InvalidChatIDErrors(t *testing.T) {
	n := NewTelegramNotifier(nil, "https://freehire.me")
	if err := n.Send(context.Background(), "telegram", "not-a-number", []ReminderMessage{{Slug: "s"}}); err == nil {
		t.Error("want an error for a non-numeric chat id")
	}
}

// batchOf builds n distinct saved jobs, so a test can push a message past its
// bounds without hand-writing the list.
func batchOf(n int) []ReminderMessage {
	ms := make([]ReminderMessage, n)
	for i := range ms {
		id := strconv.Itoa(i)
		ms[i] = ReminderMessage{JobTitle: "Job " + id, Company: "Company " + id, Slug: "job-" + id}
	}
	return ms
}

func TestEmailNotifier_BatchListsEveryJobUnderTheListLimit(t *testing.T) {
	sender := &captureSender{}
	n := NewEmailNotifier(sender, "jobs@freehire.me", "https://freehire.me")

	if err := n.Send(context.Background(), "email", "u@x.com", batchOf(4)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !strings.Contains(sender.subject, "4 saved jobs") {
		t.Errorf("subject = %q, want the batch count", sender.subject)
	}
	for i := 0; i < 4; i++ {
		if !strings.Contains(sender.html, "jobs/job-"+strconv.Itoa(i)) {
			t.Errorf("html is missing job-%d: %q", i, sender.html)
		}
	}
	if strings.Contains(sender.html, "View all") {
		t.Errorf("a batch under the list limit must show no overflow tail: %q", sender.html)
	}
}

// The two bounds are different numbers on purpose: the message itemizes
// notify.ListLimit and counts the rest, while the record behind it holds them all.
func TestEmailNotifier_BatchOverTheListLimitCountsTheRest(t *testing.T) {
	sender := &captureSender{}
	n := NewEmailNotifier(sender, "jobs@freehire.me", "https://freehire.me")

	size := notify.ListLimit + 5
	if err := n.Send(context.Background(), "email", "u@x.com", batchOf(size)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !strings.Contains(sender.html, "5 more") {
		t.Errorf("html = %q, want a tail counting the 5 omitted jobs", sender.html)
	}
	if strings.Contains(sender.html, "jobs/job-"+strconv.Itoa(notify.ListLimit)) {
		t.Errorf("html itemized past the list limit: %q", sender.html)
	}
	if !strings.Contains(sender.text, "+ 5 more") {
		t.Errorf("text = %q, want the same tail as the HTML", sender.text)
	}
}

func TestTelegramNotifier_BatchListsJobsAndCountsTheRest(t *testing.T) {
	n := NewTelegramNotifier(nil, "https://freehire.me")
	got := n.render(batchOf(notify.ListLimit + 3))

	if !strings.Contains(got, "+ 3 more") {
		t.Errorf("render = %q, want a tail counting the 3 omitted jobs", got)
	}
	if !strings.Contains(got, "jobs/job-0") {
		t.Errorf("render = %q, want the first job listed", got)
	}
}

// An oversized body is rejected deterministically and every retry re-fails, so the
// message must fit whatever the batch holds.
func TestTelegramNotifier_BatchStaysUnderTheMessageLimit(t *testing.T) {
	n := NewTelegramNotifier(nil, "https://freehire.me")
	ms := batchOf(notify.ListLimit)
	long := strings.Repeat("Extremely Long Job Title ", 40)
	for i := range ms {
		ms[i].JobTitle = long
		ms[i].Company = long
	}

	if got := telegramnotify.UTF16Len(n.render(ms)); got > telegramnotify.MaxMessageLen {
		t.Errorf("rendered %d UTF-16 units, want at most %d", got, telegramnotify.MaxMessageLen)
	}
}
