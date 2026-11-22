package feed

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCursor_RoundTripTamperExpiry(t *testing.T) {
	s := NewSigner([]byte("k"))
	now := time.Unix(1_800_000_000, 0)
	want := Cursor{UserID: 7, Source: "profile", Version: 42, SnapshotAt: now.Unix(), Offset: 20, ExpiresAt: now.Add(CursorTTL).Unix(), Phase: PhaseRest, SeenAt: now.Unix() - 60}
	tok := s.Sign(want)

	if got, err := s.Verify(tok, now); err != nil || got != want {
		t.Fatalf("round trip = %+v, %v", got, err)
	}
	other := s.Sign(Cursor{UserID: 8, Offset: 20, ExpiresAt: want.ExpiresAt})
	otherPayload, _, _ := strings.Cut(other, ".")
	_, tokTag, _ := strings.Cut(tok, ".")
	forged := otherPayload + "." + tokTag
	for name, c := range map[string]string{
		"tampered payload": "x" + tok,
		"payload swapped":  forged,
		"no dot":           "abc",
		"empty":            "",
		"flipped payload":  flip(tok),
		"other key":        NewSigner([]byte("other")).Sign(want),
	} {
		if _, err := s.Verify(c, now); !errors.Is(err, ErrBadCursor) {
			t.Errorf("%s: err = %v, want ErrBadCursor", name, err)
		}
	}
	if _, err := s.Verify(tok, now.Add(CursorTTL)); !errors.Is(err, ErrBadCursor) {
		t.Errorf("expired: err = %v", err)
	}
}

// flip changes the first payload character to a different base64url one.
func flip(tok string) string {
	c := byte('A')
	if tok[0] == 'A' {
		c = 'B'
	}
	return string(c) + tok[1:]
}
