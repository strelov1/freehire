package notify

import (
	"context"
	"errors"
	"testing"
)

// recordingNotifier captures the channel and dest it was routed, so a Router test
// can assert which underlying notifier received a send.
type recordingNotifier struct {
	channel, dest string
	sent          []Digest
}

func (n *recordingNotifier) Send(_ context.Context, channel, dest string, d Digest) error {
	n.channel, n.dest = channel, dest
	n.sent = append(n.sent, d)
	return nil
}

func TestRouter_DispatchesByChannel(t *testing.T) {
	tg := &recordingNotifier{}
	em := &recordingNotifier{}
	r := Router{ChannelTelegram: tg, ChannelEmail: em}

	if err := r.Send(context.Background(), ChannelEmail, "a@b.com", Digest{}); err != nil {
		t.Fatal(err)
	}
	if len(em.sent) != 1 || em.dest != "a@b.com" {
		t.Errorf("email notifier got %d sends dest=%q, want 1 to a@b.com", len(em.sent), em.dest)
	}
	if len(tg.sent) != 0 {
		t.Errorf("telegram notifier got %d sends, want 0 (email channel must not reach it)", len(tg.sent))
	}
}

func TestRouter_UnregisteredChannelReturnsSentinel(t *testing.T) {
	r := Router{ChannelTelegram: &recordingNotifier{}}

	err := r.Send(context.Background(), ChannelEmail, "x", Digest{})
	if !errors.Is(err, ErrChannelNotConfigured) {
		t.Errorf("err = %v, want ErrChannelNotConfigured for an unregistered channel", err)
	}
}
