package emailnotify

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sesv2"
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

	err := c.Send(context.Background(), "from@freehire.dev", "to@acme.com", "Subj", "<b>hi</b>", "hi")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if fake.in == nil {
		t.Fatal("SendEmail was not called")
	}
	if got := deref(fake.in.FromEmailAddress); got != "from@freehire.dev" {
		t.Errorf("from = %q, want from@freehire.dev", got)
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
	if err := c.Send(context.Background(), "f", "t", "s", "h", "x"); err == nil {
		t.Error("Send should propagate the SES error")
	}
}

// deref dereferences an optional *string the SDK uses, treating nil as empty.
func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
