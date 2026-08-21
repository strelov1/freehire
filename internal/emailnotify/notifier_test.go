package emailnotify

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/notify"
)

// fakeSender captures what the notifier hands to the transport, so a test can
// assert the SES call arguments without touching AWS.
type fakeSender struct {
	from, to, subject, html, text string
	calls                         int
	err                           error
}

func (s *fakeSender) Send(_ context.Context, from, to, subject, html, text string) error {
	s.calls++
	s.from, s.to, s.subject, s.html, s.text = from, to, subject, html, text
	return s.err
}

func digest() notify.Digest {
	return notify.Digest{
		SavedSearchName: "Go & <remote>",
		Total:           3,
		Jobs: []notify.DigestJob{
			{Title: "Go Dev <x>", Company: "Acme", Slug: "go-dev-acme",
				SalaryMin: 130000, SalaryMax: 170000, SalaryCurrency: "USD", SalaryPeriod: "year"},
			{Title: "Rustacean", Company: "", Slug: "rustacean-foo"},
		},
	}
}

func TestNotifier_RenderSubject(t *testing.T) {
	n := NewNotifier(&fakeSender{}, "notifications@freehire.me", "https://freehire.me/")

	got := n.render(digest())
	if want := `3 new jobs for "Go & <remote>"`; got.subject != want {
		t.Errorf("subject = %q, want %q", got.subject, want)
	}

	one := n.render(notify.Digest{SavedSearchName: "x", Total: 1, Jobs: []notify.DigestJob{{Title: "A", Slug: "a"}}})
	if want := `1 new job for "x"`; one.subject != want {
		t.Errorf("singular subject = %q, want %q", one.subject, want)
	}
}

func TestNotifier_RenderHTML(t *testing.T) {
	n := NewNotifier(&fakeSender{}, "notifications@freehire.me", "https://freehire.me/")
	got := n.render(digest()).html

	// The saved-search name and a hostile title are auto-escaped by html/template.
	if !strings.Contains(got, "Go &amp; &lt;remote&gt;") {
		t.Errorf("saved-search name not escaped: %q", got)
	}
	if !strings.Contains(got, "Go Dev &lt;x&gt;") {
		t.Errorf("job title not escaped: %q", got)
	}
	if strings.Contains(got, "<x>") {
		t.Errorf("raw unescaped title leaked into HTML: %q", got)
	}
	// Each job links to its on-platform freehire page tagged with the email UTM.
	if !strings.Contains(got, "https://freehire.me/jobs/go-dev-acme?utm_source=email") {
		t.Errorf("missing job link: %q", got)
	}
	// Company + salary render for the first job.
	if !strings.Contains(got, "Acme") || !strings.Contains(got, "$130K—$170K / year") {
		t.Errorf("missing company/salary: %q", got)
	}
	// Total 3 but only 2 listed → an "and 1 more" tail linking to the alerts page.
	if !strings.Contains(got, "1 more") {
		t.Errorf("missing overflow tail: %q", got)
	}
	if !strings.Contains(got, "https://freehire.me/my/notifications") {
		t.Errorf("missing manage-alerts footer link: %q", got)
	}
}

func TestNotifier_RenderTextAlternative(t *testing.T) {
	n := NewNotifier(&fakeSender{}, "notifications@freehire.me", "https://freehire.me")
	got := n.render(digest()).text

	// The text alternative carries the same content in plain form (unescaped).
	if !strings.Contains(got, "Go Dev <x>") {
		t.Errorf("text should carry the raw title: %q", got)
	}
	if !strings.Contains(got, "Acme") || !strings.Contains(got, "$130K—$170K / year") {
		t.Errorf("text missing company/salary: %q", got)
	}
	if !strings.Contains(got, "https://freehire.me/jobs/go-dev-acme?utm_source=email") {
		t.Errorf("text missing job link: %q", got)
	}
	if !strings.Contains(got, "1 more") {
		t.Errorf("text missing overflow tail: %q", got)
	}
}

func TestNotifier_Send(t *testing.T) {
	fs := &fakeSender{}
	n := NewNotifier(fs, "notifications@freehire.me", "https://freehire.me")

	err := n.Send(context.Background(), notify.ChannelEmail, "user@acme.com", digest())
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if fs.calls != 1 {
		t.Fatalf("sender calls = %d, want 1", fs.calls)
	}
	// From carries a display name over the configured address — see emailnotify.From.
	if !strings.Contains(fs.from, "notifications@freehire.me") || !strings.Contains(fs.from, "freehire") {
		t.Errorf("from = %q, want the configured address behind a readable name", fs.from)
	}
	if fs.to != "user@acme.com" {
		t.Errorf("to = %q, want the subscriber's address", fs.to)
	}
	if !strings.HasPrefix(fs.subject, "3 new jobs for") {
		t.Errorf("subject = %q", fs.subject)
	}
	if fs.html == "" || fs.text == "" {
		t.Errorf("both html and text bodies must be set (html=%d bytes, text=%d bytes)", len(fs.html), len(fs.text))
	}
}

func TestNotifier_SendPropagatesError(t *testing.T) {
	fs := &fakeSender{err: errors.New("ses throttled")}
	n := NewNotifier(fs, "notifications@freehire.me", "https://freehire.me")

	if err := n.Send(context.Background(), notify.ChannelEmail, "user@acme.com", digest()); err == nil {
		t.Error("Send should propagate the sender error so the delivery retries")
	}
}

// --- listing bound and the "view all" destination --------------------------

// bigDigest is a digest of n matched jobs, all recorded — the shape the engine
// now hands over once the snapshot stopped being truncated by the mail's cap.
func bigDigest(n int, notificationID int64) notify.Digest {
	d := notify.Digest{SavedSearchName: "Go", Total: n, NotificationID: notificationID}
	for i := range n {
		d.Jobs = append(d.Jobs, notify.DigestJob{Title: "Job", Company: "Acme", Slug: "job-" + strconv.Itoa(i)})
	}
	return d
}

func TestNotifier_ListsAtMostTenJobs(t *testing.T) {
	n := NewNotifier(&fakeSender{}, "notifications@freehire.me", "https://freehire.me")
	got := n.render(bigDigest(67, 42))

	if c := strings.Count(got.html, "/jobs/job-"); c != notify.ListLimit {
		t.Errorf("HTML lists %d jobs, want %d", c, notify.ListLimit)
	}
	if c := strings.Count(got.text, "/jobs/job-"); c != notify.ListLimit {
		t.Errorf("text lists %d jobs, want %d", c, notify.ListLimit)
	}
	if !strings.Contains(got.html, "57 more") || !strings.Contains(got.text, "57 more") {
		t.Errorf("missing the 57-more tail:\nhtml=%s\ntext=%s", got.html, got.text)
	}
	if want := `67 new jobs for "Go"`; got.subject != want {
		t.Errorf("subject = %q, want %q — the count is the news, only the body is bounded", got.subject, want)
	}
}

func TestNotifier_TailLinksToTheDigestsOwnPage(t *testing.T) {
	n := NewNotifier(&fakeSender{}, "notifications@freehire.me", "https://freehire.me")
	got := n.render(bigDigest(67, 42))

	const want = "https://freehire.me/my/notifications/42/jobs?utm_source=email"
	if !strings.Contains(got.html, want) {
		t.Errorf("HTML tail does not link to %q: %s", want, got.html)
	}
	if !strings.Contains(got.text, want) {
		t.Errorf("text tail does not link to %q: %s", want, got.text)
	}
}

func TestNotifier_TailFallsBackWithoutANotificationID(t *testing.T) {
	n := NewNotifier(&fakeSender{}, "notifications@freehire.me", "https://freehire.me")
	got := n.render(bigDigest(67, 0))

	if strings.Contains(got.html, "/my/notifications/0/jobs") {
		t.Errorf("a zero notification id must not be rendered as a URL: %s", got.html)
	}
	if !strings.Contains(got.html, "https://freehire.me/my/notifications") {
		t.Errorf("missing the fallback destination: %s", got.html)
	}
	if !strings.Contains(got.text, "57 more") {
		t.Errorf("the tail must still render without an id: %s", got.text)
	}
}
