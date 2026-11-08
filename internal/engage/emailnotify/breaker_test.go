package emailnotify

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"

	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

type countingSES struct {
	fakeSES
	mu    sync.Mutex
	calls int
}

func (f *countingSES) SendEmail(ctx context.Context, in *sesv2.SendEmailInput, o ...func(*sesv2.Options)) (*sesv2.SendEmailOutput, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return f.fakeSES.SendEmail(ctx, in, o...)
}

func essentialMail() Message {
	return Message{From: "a@freehire.me", To: "b@acme.com", Subject: "s", HTML: "h", Text: "t", Group: emailprefs.GroupEssential}
}

func TestBreakerTripsOnAccountLevelRefusals(t *testing.T) {
	for name, refusal := range map[string]error{
		"sending paused exception": &types.SendingPausedException{Message: aws.String("paused")},
		"account suspended":        &types.AccountSuspendedException{Message: aws.String("suspended")},
		"message rejected, paused": &types.MessageRejected{Message: aws.String("Sending paused for this account.")},
	} {
		t.Run(name, func(t *testing.T) {
			fake := &countingSES{fakeSES: fakeSES{err: refusal}}
			c := (&Client{ses: fake}).WithSendingBreaker()

			first := c.Send(context.Background(), essentialMail())
			if !errors.Is(first, ErrSendingPaused) {
				t.Fatalf("first Send = %v, want ErrSendingPaused", first)
			}
			second := c.Send(context.Background(), essentialMail())
			if !errors.Is(second, ErrSendingPaused) {
				t.Fatalf("second Send = %v, want ErrSendingPaused", second)
			}
			if fake.calls != 1 {
				t.Errorf("SES calls = %d, want 1: a tripped breaker must not call SES", fake.calls)
			}
		})
	}
}

func TestBreakerIgnoresPerAddressRejection(t *testing.T) {
	fake := &countingSES{fakeSES: fakeSES{err: &types.MessageRejected{Message: aws.String("Email address is not verified.")}}}
	c := (&Client{ses: fake}).WithSendingBreaker()

	for i := 0; i < 2; i++ {
		err := c.Send(context.Background(), essentialMail())
		if err == nil || errors.Is(err, ErrSendingPaused) {
			t.Fatalf("Send %d = %v, want a plain rejection", i, err)
		}
	}
	if fake.calls != 2 {
		t.Errorf("SES calls = %d, want 2: one bad address must not stop the rest", fake.calls)
	}
}

func TestClientWithoutBreakerNeverTrips(t *testing.T) {
	fake := &countingSES{fakeSES: fakeSES{err: &types.SendingPausedException{Message: aws.String("paused")}}}
	c := &Client{ses: fake}

	for i := 0; i < 2; i++ {
		if err := c.Send(context.Background(), essentialMail()); errors.Is(err, ErrSendingPaused) {
			t.Fatalf("Send %d returned ErrSendingPaused from a client without the breaker", i)
		}
	}
	if fake.calls != 2 {
		t.Errorf("SES calls = %d, want 2", fake.calls)
	}
}

func TestBreakerSurvivesEngagementTracking(t *testing.T) {
	fake := &countingSES{fakeSES: fakeSES{err: &types.SendingPausedException{Message: aws.String("paused")}}}
	c := (&Client{ses: fake}).WithSendingBreaker().WithEngagementTracking("set")

	_ = c.Send(context.Background(), essentialMail())
	if err := c.Send(context.Background(), essentialMail()); !errors.Is(err, ErrSendingPaused) {
		t.Fatalf("second Send = %v, want ErrSendingPaused", err)
	}
}
