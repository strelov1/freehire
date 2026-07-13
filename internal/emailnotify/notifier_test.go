package emailnotify

import (
	"context"
	"errors"
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
	n := NewNotifier(&fakeSender{}, "notifications@freehire.dev", "https://freehire.dev/")

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
	n := NewNotifier(&fakeSender{}, "notifications@freehire.dev", "https://freehire.dev/")
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
	if !strings.Contains(got, "https://freehire.dev/jobs/go-dev-acme?utm_source=email") {
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
	if !strings.Contains(got, "https://freehire.dev/my/notifications") {
		t.Errorf("missing manage-alerts footer link: %q", got)
	}
}

func TestNotifier_RenderTextAlternative(t *testing.T) {
	n := NewNotifier(&fakeSender{}, "notifications@freehire.dev", "https://freehire.dev")
	got := n.render(digest()).text

	// The text alternative carries the same content in plain form (unescaped).
	if !strings.Contains(got, "Go Dev <x>") {
		t.Errorf("text should carry the raw title: %q", got)
	}
	if !strings.Contains(got, "Acme") || !strings.Contains(got, "$130K—$170K / year") {
		t.Errorf("text missing company/salary: %q", got)
	}
	if !strings.Contains(got, "https://freehire.dev/jobs/go-dev-acme?utm_source=email") {
		t.Errorf("text missing job link: %q", got)
	}
	if !strings.Contains(got, "1 more") {
		t.Errorf("text missing overflow tail: %q", got)
	}
}

func TestNotifier_Send(t *testing.T) {
	fs := &fakeSender{}
	n := NewNotifier(fs, "notifications@freehire.dev", "https://freehire.dev")

	err := n.Send(context.Background(), notify.ChannelEmail, "user@acme.com", digest())
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if fs.calls != 1 {
		t.Fatalf("sender calls = %d, want 1", fs.calls)
	}
	if fs.from != "notifications@freehire.dev" || fs.to != "user@acme.com" {
		t.Errorf("from/to = %q/%q, want notifications@freehire.dev/user@acme.com", fs.from, fs.to)
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
	n := NewNotifier(fs, "notifications@freehire.dev", "https://freehire.dev")

	if err := n.Send(context.Background(), notify.ChannelEmail, "user@acme.com", digest()); err == nil {
		t.Error("Send should propagate the sender error so the delivery retries")
	}
}
