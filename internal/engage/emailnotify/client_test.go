package emailnotify

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sesv2"

	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

// fakeSES records the SendEmail input and returns a configurable error, so the
// Client adapter is verified without a live AWS call.
type fakeSES struct {
	in  *sesv2.SendEmailInput
	err error
}

func (f *fakeSES) SendEmail(_ context.Context, in *sesv2.SendEmailInput, _ ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
	f.in = in
	if f.err != nil {
		return nil, f.err
	}
	return &sesv2.SendEmailOutput{}, nil
}

func TestClient_SendBuildsSimpleEmail(t *testing.T) {
	fake := &fakeSES{}
	c := &Client{ses: fake}

	err := c.Send(context.Background(), Message{
		From: "from@freehire.me", To: "to@acme.com", Subject: "Subj",
		HTML: "<b>hi</b>", Text: "hi", Group: emailprefs.GroupEssential,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if fake.in == nil {
		t.Fatal("SendEmail was not called")
	}
	if got := deref(fake.in.FromEmailAddress); got != "from@freehire.me" {
		t.Errorf("from = %q, want from@freehire.me", got)
	}
	if len(fake.in.Destination.ToAddresses) != 1 || fake.in.Destination.ToAddresses[0] != "to@acme.com" {
		t.Errorf("to = %v, want [to@acme.com]", fake.in.Destination.ToAddresses)
	}
	simple := fake.in.Content.Simple
	if simple == nil {
		t.Fatal("Content.Simple is nil")
	}
	if got := deref(simple.Subject.Data); got != "Subj" {
		t.Errorf("subject = %q, want Subj", got)
	}
	if got := deref(simple.Body.Html.Data); got != "<b>hi</b>" {
		t.Errorf("html = %q, want <b>hi</b>", got)
	}
	if got := deref(simple.Body.Text.Data); got != "hi" {
		t.Errorf("text = %q, want hi", got)
	}
}

func TestClient_SendPropagatesError(t *testing.T) {
	c := &Client{ses: &fakeSES{err: errors.New("throttled")}}
	m := Message{From: "f", To: "t", Subject: "s", HTML: "h", Text: "x", Group: emailprefs.GroupEssential}
	if err := c.Send(context.Background(), m); err == nil {
		t.Error("Send should propagate the SES error")
	}
}

// The choke point the change hangs on: a mail somebody may turn off, carrying no way
// to turn it off, is refused rather than sent. This is what covers the mails that
// build their own HTML and never touch the shared template.
func TestClient_RefusesASilenceableMailWithNoWayOut(t *testing.T) {
	fake := &fakeSES{}
	c := &Client{ses: fake}

	cases := []struct {
		name string
		m    Message
	}{
		{"no group at all", Message{From: "f", To: "t", Subject: "s", HTML: "h", Text: "x"}},
		{"silenceable, no URL", Message{From: "f", To: "t", Subject: "s", HTML: "h", Text: "x", Group: emailprefs.GroupNews}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := c.Send(context.Background(), tc.m); !errors.Is(err, ErrNoWayOut) {
				t.Errorf("Send gave %v, want ErrNoWayOut", err)
			}
		})
	}
	if fake.in != nil {
		t.Error("SendEmail was called for a message that should never have left")
	}
}

// The mirror of the rule above: an essential mail must not advertise a control we
// cannot honour, because cancelling a password-reset mail is not a thing.
func TestClient_RefusesAnEssentialMailCarryingAnUnsubscribeURL(t *testing.T) {
	fake := &fakeSES{}
	c := &Client{ses: fake}
	m := Message{
		From: "f", To: "t", Subject: "s", HTML: "h", Text: "x",
		Group: emailprefs.GroupEssential, UnsubscribeURL: "https://freehire.me/unsubscribe?t=x",
	}
	if err := c.Send(context.Background(), m); err == nil {
		t.Error("an essential mail carrying an unsubscribe URL should be refused")
	}
	if fake.in != nil {
		t.Error("SendEmail was called for a message that should never have left")
	}
}

// RFC 8058: Gmail POSTs to the URL and reports the person unsubscribed, so both
// headers have to be present and spelled exactly.
func TestClient_SendsTheOneClickHeaders(t *testing.T) {
	fake := &fakeSES{}
	c := &Client{ses: fake}
	const url = "https://freehire.me/unsubscribe?t=abc"

	err := c.Send(context.Background(), Message{
		From: "f", To: "t", Subject: "s", HTML: "h", Text: "x",
		Group: emailprefs.GroupAlerts, UnsubscribeURL: url,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	got := map[string]string{}
	for _, h := range fake.in.Content.Simple.Headers {
		got[deref(h.Name)] = deref(h.Value)
	}
	if want := "<" + url + ">"; got["List-Unsubscribe"] != want {
		t.Errorf("List-Unsubscribe = %q, want %q", got["List-Unsubscribe"], want)
	}
	if want := "List-Unsubscribe=One-Click"; got["List-Unsubscribe-Post"] != want {
		t.Errorf("List-Unsubscribe-Post = %q, want %q", got["List-Unsubscribe-Post"], want)
	}
}

// An essential mail carries neither header, so a client shows no unsubscribe control
// on a message that has none.
func TestClient_SendsNoUnsubscribeHeadersOnEssentialMail(t *testing.T) {
	fake := &fakeSES{}
	c := &Client{ses: fake}

	err := c.Send(context.Background(), Message{
		From: "f", To: "t", Subject: "s", HTML: "h", Text: "x", Group: emailprefs.GroupEssential,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if hs := fake.in.Content.Simple.Headers; len(hs) != 0 {
		t.Errorf("essential mail carried %d headers, want none", len(hs))
	}
}

// deref dereferences an optional *string the SDK uses, treating nil as empty.
func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
