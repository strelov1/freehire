package oauth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

// MobileCallbackURL is the custom-scheme deep link the mobile app registers.
// The native OAuth flow finishes by redirecting here with a one-time `?code=…`
// (or `?auth_error=oauth`); the app then exchanges the code for a session over
// its own HTTP client, so the session cookie lands in the app's cookie jar.
const MobileCallbackURL = "freehiremobile://auth-callback"

// codeEntry is a minted one-time code's payload: the user it authenticates and
// when it stops being valid.
type codeEntry struct {
	userID  int64
	expires time.Time
}

// CodeStore hands out single-use, short-lived codes that stand in for a freshly
// authenticated session during the mobile OAuth handshake. It is in-memory:
// good for a single instance (the exchange happens seconds after minting, on
// the same client). Behind a horizontally-scaled deployment it would need a
// shared backing (sticky sessions, Redis, or a DB table) so the exchange can
// hit the instance that minted the code.
type CodeStore struct {
	mu    sync.Mutex
	codes map[string]codeEntry
	ttl   time.Duration
	now   func() time.Time // injectable for tests
}

// NewCodeStore returns an empty store whose codes live for ttl.
func NewCodeStore(ttl time.Duration) *CodeStore {
	return &CodeStore{codes: make(map[string]codeEntry), ttl: ttl, now: time.Now}
}

// Mint returns a fresh random code bound to userID, valid for the store's TTL.
// It opportunistically drops expired entries so the map can't grow unbounded
// from codes that are never exchanged.
func (s *CodeStore) Mint(userID int64) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	code := base64.RawURLEncoding.EncodeToString(b)

	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for k, e := range s.codes {
		if !e.expires.After(now) {
			delete(s.codes, k)
		}
	}
	s.codes[code] = codeEntry{userID: userID, expires: now.Add(s.ttl)}
	return code, nil
}

// Consume redeems a code exactly once: it returns the bound user id and deletes
// the code, or reports ok=false when the code is unknown, already used, or
// expired.
func (s *CodeStore) Consume(code string) (int64, bool) {
	if code == "" {
		return 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.codes[code]
	if !ok {
		return 0, false
	}
	delete(s.codes, code) // single-use: gone whether or not it had expired
	if !e.expires.After(s.now()) {
		return 0, false
	}
	return e.userID, true
}
