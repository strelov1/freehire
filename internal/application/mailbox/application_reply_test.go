package mailbox

import (
	"testing"

	"github.com/google/uuid"
)

func TestApplicationReplyAddressRoundTrips(t *testing.T) {
	token := uuid.New()
	addr := ApplicationReplyAddress(token, "mail.example.test")
	got, ok := ApplicationReplyToken(addr)
	if !ok || got != token {
		t.Fatalf("ApplicationReplyToken(%q) = %v, %v", addr, got, ok)
	}
}

func TestApplicationReplyTokenRejectsNonCanonical(t *testing.T) {
	token := uuid.New()
	for _, addr := range []string{
		"alice@mail.example.test",
		"ats-reply-not-a-uuid@mail.example.test",
		"ats-reply-" + uuid.Nil.String() + "@mail.example.test",
		"ats-reply-" + token.String(), // no domain
		"ats-reply-" + uuid.UUID(token).URN() + "@mail.example.test",
	} {
		if _, ok := ApplicationReplyToken(addr); ok {
			t.Errorf("ApplicationReplyToken(%q) accepted", addr)
		}
	}
}
