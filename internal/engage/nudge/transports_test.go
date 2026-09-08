package nudge

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/engage/telegramnotify"
	"github.com/strelov1/freehire/internal/platform/db"
)

// captureSender records the last email an EmailNotifier sent.
type captureSender struct {
	subject, html, text string
}

func (s *captureSender) Send(_ context.Context, _, _, subject, html, text string) error {
	s.subject, s.html, s.text = subject, html, text
	return nil
}

// batchOf builds n distinct nudges of one kind, so a test can push a message past
// its bounds without hand-writing the list.
func batchOf(kind string, n int) []Message {
	ms := make([]Message, n)
	for i := range ms {
		id := strconv.Itoa(i)
		ms[i] = Message{Kind: kind, JobTitle: "Job " + id, Company: "Company " + id, Slug: "job-" + id, DaysSilent: 20 + i}
	}
	return ms
}

func TestEmailNotifier_BatchListsEveryNudgeUnderTheListLimit(t *testing.T) {
	sender := &captureSender{}
	n := NewEmailNotifier(sender, "jobs@freehire.me", "https://freehire.me")

	if err := n.Send(context.Background(), "email", "u@x.com", KindFollowUp, batchOf(KindFollowUp, 3)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !strings.Contains(sender.subject, "3 applications") {
		t.Errorf("subject = %q, want the batch count", sender.subject)
	}
	for i := 0; i < 3; i++ {
		if !strings.Contains(sender.html, "jobs/job-"+strconv.Itoa(i)) {
			t.Errorf("html is missing job-%d: %q", i, sender.html)
		}
	}
	if !strings.Contains(sender.html, "/my/tracking") {
		t.Errorf("a follow-up batch must lead to the tracking board: %q", sender.html)
	}
}

// job-closed has nothing left to track, so its batch leads to the saved list.
func TestEmailNotifier_JobClosedBatchLeadsToSavedJobs(t *testing.T) {
	sender := &captureSender{}
	n := NewEmailNotifier(sender, "jobs@freehire.me", "https://freehire.me")

	if err := n.Send(context.Background(), "email", "u@x.com", KindJobClosed, batchOf(KindJobClosed, 2)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !strings.Contains(sender.html, "/my/activity") {
		t.Errorf("html = %q, want the saved-jobs destination", sender.html)
	}
}

// interview-prep is about the application too, so its batch leads to the tracking
// board — the third kind, and the one neither of the two above covers.
func TestEmailNotifier_InterviewPrepBatchLeadsToTheTrackingBoard(t *testing.T) {
	sender := &captureSender{}
	n := NewEmailNotifier(sender, "jobs@freehire.me", "https://freehire.me")

	if err := n.Send(context.Background(), "email", "u@x.com", KindInterviewPrep, batchOf(KindInterviewPrep, 2)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !strings.Contains(sender.subject, "2 interviews") {
		t.Errorf("subject = %q, want the batch count", sender.subject)
	}
	if !strings.Contains(sender.html, "/my/tracking") {
		t.Errorf("html = %q, want the tracking-board destination", sender.html)
	}
}

// follow-up and interview-prep single-message notifications still lead to the
// general tracking board — only the three auto-apply outcome kinds deep-link.
func TestEmailNotifier_FollowUpAndInterviewPrepSingle_LeadToTheGeneralBoard(t *testing.T) {
	for _, kind := range []string{KindFollowUp, KindInterviewPrep} {
		t.Run(kind, func(t *testing.T) {
			sender := &captureSender{}
			n := NewEmailNotifier(sender, "jobs@freehire.me", "https://freehire.me")
			ms := []Message{{Kind: kind, JobTitle: "Go Dev", Company: "Acme", Slug: "go-dev-acme"}}
			if err := n.Send(context.Background(), "email", "u@x.com", kind, ms); err != nil {
				t.Fatalf("Send: %v", err)
			}
			if !strings.Contains(sender.html, "https://freehire.me/my/tracking?utm_source=email") {
				t.Errorf("html = %q, want the bare tracking-board destination", sender.html)
			}
			if strings.Contains(sender.html, "/my/tracking/go-dev-acme") {
				t.Errorf("html = %q, must not deep-link to the specific application", sender.html)
			}
		})
	}
}

// The three auto-apply outcome kinds are about an application, same as
// follow-up/interview-prep, so a batch leads to the tracking board — never
// /my/activity, unlike job-closed. A single-application notification, unlike
// follow-up/interview-prep, deep-links straight to that application's drawer.
func TestEmailNotifier_AutoApplyOutcomeKinds_SingleAndBatch(t *testing.T) {
	cases := []struct {
		kind        string
		wantSubject string
	}{
		{KindAutoApplySubmitted, "Submitted"},
		{KindAutoApplyBlocked, "Needs your attention"},
		{KindAutoApplyFailed, "Couldn't submit"},
	}
	for _, c := range cases {
		t.Run(c.kind+"/single", func(t *testing.T) {
			sender := &captureSender{}
			n := NewEmailNotifier(sender, "jobs@freehire.me", "https://freehire.me")
			ms := []Message{{Kind: c.kind, JobTitle: "Go Dev", Company: "Acme", Slug: "go-dev-acme"}}
			if err := n.Send(context.Background(), "email", "u@x.com", c.kind, ms); err != nil {
				t.Fatalf("Send: %v", err)
			}
			if !strings.Contains(sender.subject, c.wantSubject) {
				t.Errorf("subject = %q, want it to contain %q", sender.subject, c.wantSubject)
			}
			if !strings.Contains(sender.html, "/my/tracking/go-dev-acme") {
				t.Errorf("html = %q, want a deep link to the specific application", sender.html)
			}
		})
		t.Run(c.kind+"/batch", func(t *testing.T) {
			sender := &captureSender{}
			n := NewEmailNotifier(sender, "jobs@freehire.me", "https://freehire.me")
			if err := n.Send(context.Background(), "email", "u@x.com", c.kind, batchOf(c.kind, 2)); err != nil {
				t.Fatalf("Send: %v", err)
			}
			if !strings.Contains(sender.subject, "2") {
				t.Errorf("subject = %q, want the batch count", sender.subject)
			}
			if !strings.Contains(sender.html, "https://freehire.me/my/tracking?utm_source=email") {
				t.Errorf("html = %q, want the bare tracking-board destination for a batch", sender.html)
			}
		})
	}
}

func TestTelegramNotifier_AutoApplyOutcomeKinds_SingleAndBatch(t *testing.T) {
	for _, kind := range []string{KindAutoApplySubmitted, KindAutoApplyBlocked, KindAutoApplyFailed} {
		t.Run(kind+"/single", func(t *testing.T) {
			n := NewTelegramNotifier(nil, "https://freehire.me")
			got := n.render(kind, []Message{{Kind: kind, JobTitle: "Go Dev", Company: "Acme", Slug: "go-dev-acme"}})
			if !strings.Contains(got, "Go Dev") || !strings.Contains(got, "Acme") {
				t.Errorf("render = %q, want the job title and company", got)
			}
			if !strings.Contains(got, "/my/tracking/go-dev-acme") {
				t.Errorf("render = %q, want a deep link to the specific application", got)
			}
		})
		t.Run(kind+"/batch", func(t *testing.T) {
			n := NewTelegramNotifier(nil, "https://freehire.me")
			got := n.render(kind, batchOf(kind, 2))
			if !strings.Contains(got, "<b>2</b>") {
				t.Errorf("render = %q, want the batch count", got)
			}
			// A batch within the list limit names each job individually (jobLine),
			// never a per-application deep link — batchDestination's bare-board link
			// only appears in the overflow tail, exercised by the "+more" tests below.
		})
	}
}

// follow-up and interview-prep single-message notifications still lead to the
// general tracking board — only the three auto-apply outcome kinds deep-link.
func TestTelegramNotifier_FollowUpAndInterviewPrepSingle_LeadToTheGeneralBoard(t *testing.T) {
	for _, kind := range []string{KindFollowUp, KindInterviewPrep} {
		t.Run(kind, func(t *testing.T) {
			n := NewTelegramNotifier(nil, "https://freehire.me")
			got := n.render(kind, []Message{{Kind: kind, JobTitle: "Go Dev", Company: "Acme", Slug: "go-dev-acme"}})
			if !strings.Contains(got, "https://freehire.me/my/tracking\"") {
				t.Errorf("render = %q, want the bare tracking-board destination", got)
			}
			if strings.Contains(got, "/my/tracking/go-dev-acme") {
				t.Errorf("render = %q, must not deep-link to the specific application", got)
			}
		})
	}
}

func TestTelegramNotifier_InterviewPrepBatchHeadlinesTheCount(t *testing.T) {
	n := NewTelegramNotifier(nil, "https://freehire.me")
	got := n.render(KindInterviewPrep, batchOf(KindInterviewPrep, 2))

	if !strings.Contains(got, "interviewing for <b>2</b> roles") {
		t.Errorf("render = %q, want the interview-prep headline with a count", got)
	}
	if !strings.Contains(got, "jobs/job-0") {
		t.Errorf("render = %q, want the jobs listed", got)
	}
}

// The mail's button and the bot's tail read one rule, so they cannot point different
// ways for the same kind.
func TestBatchDestination_IsOneRuleForBothChannels(t *testing.T) {
	for _, kind := range []string{
		KindFollowUp, KindInterviewPrep, KindJobClosed,
		KindAutoApplySubmitted, KindAutoApplyBlocked, KindAutoApplyFailed,
	} {
		path, _ := batchDestination(kind)
		tg := NewTelegramNotifier(nil, "https://freehire.me").batchURL(kind)
		mail, _ := NewEmailNotifier(&captureSender{}, "j@f.me", "https://freehire.me").batchCTA(kind)
		if tg != "https://freehire.me"+path || !strings.HasPrefix(mail, "https://freehire.me"+path) {
			t.Errorf("%s: telegram %q and email %q disagree with %q", kind, tg, mail, path)
		}
	}
}

// The two bounds are different numbers on purpose: the message itemizes
// notify.ListLimit and counts the rest.
func TestEmailNotifier_BatchOverTheListLimitCountsTheRest(t *testing.T) {
	sender := &captureSender{}
	n := NewEmailNotifier(sender, "jobs@freehire.me", "https://freehire.me")

	if err := n.Send(context.Background(), "email", "u@x.com", KindFollowUp, batchOf(KindFollowUp, notify.ListLimit+4)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !strings.Contains(sender.html, "4 more") {
		t.Errorf("html = %q, want a tail counting the 4 omitted jobs", sender.html)
	}
	if strings.Contains(sender.html, "jobs/job-"+strconv.Itoa(notify.ListLimit)) {
		t.Errorf("html itemized past the list limit: %q", sender.html)
	}
	if !strings.Contains(sender.text, "+ 4 more") {
		t.Errorf("text = %q, want the same tail as the HTML", sender.text)
	}
}

// A batch of one must be indistinguishable from what shipped before grouping.
func TestEmailNotifier_SingleNudgeKeepsItsOwnWording(t *testing.T) {
	sender := &captureSender{}
	n := NewEmailNotifier(sender, "jobs@freehire.me", "https://freehire.me")

	ms := []Message{{Kind: KindFollowUp, JobTitle: "Go Dev", Company: "Acme", Slug: "go-dev-acme", DaysSilent: 12}}
	if err := n.Send(context.Background(), "email", "u@x.com", KindFollowUp, ms); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if sender.subject != "Time to follow up: Go Dev at Acme" {
		t.Errorf("subject = %q, want the single-nudge line", sender.subject)
	}
}

func TestTelegramNotifier_BatchListsJobsAndCountsTheRest(t *testing.T) {
	n := NewTelegramNotifier(nil, "https://freehire.me")
	got := n.render(KindFollowUp, batchOf(KindFollowUp, notify.ListLimit+2))

	if !strings.Contains(got, "+ 2 more") {
		t.Errorf("render = %q, want a tail counting the 2 omitted jobs", got)
	}
	if !strings.Contains(got, "jobs/job-0") {
		t.Errorf("render = %q, want the first job listed", got)
	}
}

// An oversized body is rejected deterministically and every retry re-fails, so the
// message must fit whatever the batch holds.
func TestTelegramNotifier_BatchStaysUnderTheMessageLimit(t *testing.T) {
	n := NewTelegramNotifier(nil, "https://freehire.me")
	ms := batchOf(KindFollowUp, notify.ListLimit)
	long := strings.Repeat("Extremely Long Job Title ", 40)
	for i := range ms {
		ms[i].JobTitle = long
		ms[i].Company = long
	}

	if got := telegramnotify.UTF16Len(n.render(KindFollowUp, ms)); got > telegramnotify.MaxMessageLen {
		t.Errorf("rendered %d UTF-16 units, want at most %d", got, telegramnotify.MaxMessageLen)
	}
}

func TestTelegramNotifier_BatchEscapesSourceData(t *testing.T) {
	n := NewTelegramNotifier(nil, "https://freehire.me")
	ms := batchOf(KindFollowUp, 2)
	ms[0].JobTitle = "<script>x</script>"

	if got := n.render(KindFollowUp, ms); strings.Contains(got, "<script>") {
		t.Errorf("title must be HTML-escaped in the body, got %q", got)
	}
}

func TestPushNotifier_BatchIsOneNotificationWithoutADeepLink(t *testing.T) {
	lister := &fakePushTokenLister{tokens: map[int64][]db.UserPushToken{42: {{Token: "tok-1"}}}}
	transport := &fakePushTransport{}
	n := NewPushNotifier(lister, transport)

	if err := n.Send(context.Background(), "push", "42", KindFollowUp, batchOf(KindFollowUp, 3)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(transport.sent) != 1 {
		t.Fatalf("sent = %d, want 1 for one device and one batch", len(transport.sent))
	}
	got := transport.sent[0]
	if !strings.Contains(got.body, "3 of your applications") {
		t.Errorf("body = %q, want the batch count", got.body)
	}
	if got.data["slug"] != "" {
		t.Errorf("data[slug] = %q, want no deep link for a multi-job batch", got.data["slug"])
	}
}
