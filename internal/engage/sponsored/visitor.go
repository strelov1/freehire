package sponsored

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// VisitorHash identifies one visitor for a day without storing anything: the salt is derived
// from the click key and the UTC date, so it rotates daily and hashes cannot be linked across
// days.
func VisitorHash(key []byte, ip, userAgent string, now time.Time) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte("visitor-salt:" + now.UTC().Format("2006-01-02")))
	h := sha256.New()
	h.Write([]byte(ip + "\x00" + userAgent + "\x00"))
	h.Write(m.Sum(nil))
	return hex.EncodeToString(h.Sum(nil))
}

// IPLimiter admits at most limit hits per window per IP (fixed window). It lives in one
// process: with several API replicas the effective limit is limit per replica.
type IPLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string]ipWindow
}

type ipWindow struct {
	start time.Time
	n     int
}

// maxTrackedIPs bounds memory: past it, windows that have ended are dropped.
const maxTrackedIPs = 10000

func NewIPLimiter(limit int, window time.Duration) *IPLimiter {
	return &IPLimiter{limit: limit, window: window, hits: map[string]ipWindow{}}
}

// Allow counts one hit from ip and reports whether it is within the limit.
func (l *IPLimiter) Allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	w, ok := l.hits[ip]
	if !ok || now.Sub(w.start) >= l.window {
		if len(l.hits) >= maxTrackedIPs {
			for k, v := range l.hits {
				if now.Sub(v.start) >= l.window {
					delete(l.hits, k)
				}
			}
		}
		w = ipWindow{start: now}
	}
	w.n++
	l.hits[ip] = w
	return w.n <= l.limit
}
