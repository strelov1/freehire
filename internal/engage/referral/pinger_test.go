package referral

import (
	"context"
	"html/template"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
)

type pingerSender struct {
	calls int
}

func (s *pingerSender) Send(context.Context, emailnotify.Message) error {
	s.calls++
	return nil
}

type pingerTelegram struct {
	calls  int
	chatID int64
}

func (t *pingerTelegram) SendMessage(_ context.Context, chatID int64, _ string) error {
	t.calls++
	t.chatID = chatID
	return nil
}

// A mail whose body failed to render must not be sent, but the notice is per
// channel: the referrer's Telegram chat still hears about the request, and the
// error is joined so the service logs it.
func TestChannelPinger_RenderFailureSkipsOnlyTheEmail(t *testing.T) {
	email := &pingerSender{}
	tg := &pingerTelegram{}
	p := NewChannelPinger(email, "hi@freehire.me", tg, "https://freehire.me",
		emailprefs.NewLinks("pinger-test-secret-padded-to-32-byte", "https://freehire.me"), nil)
	r := Recipient{UserID: 7, Email: "referrer@example.com", ChatID: 42}

	// A missing partial is accepted by Parse and reported only by Execute.
	previous := requestHTML
	t.Cleanup(func() { requestHTML = previous })
	requestHTML = template.Must(template.New("referralRequest").Parse(`{{template "no-such-partial" .}}`))

	err := p.PingReferrer(context.Background(), r, "https://freehire.me/my/referrals/incoming")
	if err == nil || !strings.Contains(err.Error(), "no-such-partial") {
		t.Fatalf("PingReferrer = %v, want the template error", err)
	}
	if email.calls != 0 {
		t.Errorf("email sent %d times, want 0: a mail whose body failed to render must not go out", email.calls)
	}
	if tg.calls != 1 || tg.chatID != 42 {
		t.Errorf("telegram calls = %d (chat %d), want 1 to chat 42: a failed email must not silence the other channel", tg.calls, tg.chatID)
	}
}
