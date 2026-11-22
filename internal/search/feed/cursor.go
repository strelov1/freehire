package feed

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// CursorTTL is how long a feed session may keep paging before it must restart (AD-7).
const CursorTTL = 24 * time.Hour

// ErrBadCursor is every reason a cursor is refused: malformed, bad signature, expired, or
// issued to another user or source. The caller answers 400 and the client restarts.
var ErrBadCursor = errors.New("feed: invalid cursor")

// Cursor is the paging state a response hands back. Clients never parse it.
type Cursor struct {
	UserID int64  `json:"u"`
	Source string `json:"s"`
	// Version is the source's filter version (the profile's UpdatedAt, in microseconds);
	// a different value means the filters changed mid-session.
	Version    int64 `json:"v"`
	SnapshotAt int64 `json:"t"` // unix seconds
	Offset     int   `json:"o"`
	ExpiresAt  int64 `json:"e"` // unix seconds
	// Phase is the segment the offset counts in: PhaseNew, PhaseRest, or PhaseSeen for the
	// cursor that only POST /me/feed/seen accepts. A cursor with no phase predates phases and
	// is refused (400), so the client restarts from page 1.
	Phase string `json:"p,omitempty"`
	// SeenAt is the feed watermark the session started from (unix seconds): new cards are
	// created after it, so it stays fixed for the whole session.
	SeenAt int64 `json:"w,omitempty"`
}

const (
	PhaseNew  = "n"
	PhaseRest = "r"
	PhaseSeen = "s"
)

// Signer signs and verifies cursors with a server secret.
type Signer struct{ key []byte }

// NewSigner builds a Signer; key must be a server secret.
func NewSigner(key []byte) *Signer { return &Signer{key: key} }

func (s *Signer) tag(payload string) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte("feed-cursor:"))
	m.Write([]byte(payload))
	return m.Sum(nil)
}

// Sign encodes c as base64url(payload) "." base64url(HMAC tag).
func (s *Signer) Sign(c Cursor) string {
	raw, _ := json.Marshal(c) // a struct of scalars cannot fail
	p := base64.RawURLEncoding.EncodeToString(raw)
	return p + "." + base64.RawURLEncoding.EncodeToString(s.tag(p))
}

// Verify returns the cursor, or ErrBadCursor when it is malformed, forged or expired at now.
func (s *Signer) Verify(str string, now time.Time) (Cursor, error) {
	p, sig, ok := strings.Cut(str, ".")
	if !ok {
		return Cursor{}, ErrBadCursor
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(got, s.tag(p)) {
		return Cursor{}, ErrBadCursor
	}
	raw, err := base64.RawURLEncoding.DecodeString(p)
	if err != nil {
		return Cursor{}, ErrBadCursor
	}
	var c Cursor
	if json.Unmarshal(raw, &c) != nil || c.Offset < 0 || now.Unix() >= c.ExpiresAt {
		return Cursor{}, ErrBadCursor
	}
	return c, nil
}
