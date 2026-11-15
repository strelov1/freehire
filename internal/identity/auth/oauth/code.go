package oauth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

// MobileCallbackURL is the app's deep link. The flow ends here with a one-time `?code=…`
// (or `?auth_error=oauth`) that the app exchanges over its own HTTP client, so the
// session cookie lands in its cookie jar.
const MobileCallbackURL = "freehiremobile://auth-callback"

// codeEntry is a code's payload: the user and expiry.
type codeEntry struct {
	userID  int64
	expires time.Time
}

// CodeStore hands out single-use, short-lived codes standing in for an authenticated
// session during the mobile handshake. In-memory, so single-instance only: a scaled
// deployment needs a shared backing so the exchange reaches the minting instance.
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

// Mint returns a random code bound to userID. It drops expired entries so unexchanged
// codes cannot grow the map.
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

// Consume redeems a code once, returning the user id; ok=false when unknown, used or expired.
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
