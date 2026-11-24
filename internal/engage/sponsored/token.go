package sponsored

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Placements a click token can name.
const (
	PlacementSearch  = "search"
	PlacementEmail   = "email"
	PlacementCompany = "company"
)

// Token lifetimes (design D7): an email is opened days after it is sent.
const (
	SearchTokenTTL = 7 * 24 * time.Hour
	EmailTokenTTL  = 30 * 24 * time.Hour
)

// ErrBadToken covers every refusal: the redirect endpoint treats them all alike.
var ErrBadToken = errors.New("sponsored: invalid click token")

// Claims is what a click token carries.
type Claims struct {
	CampaignID, JobID int64
	Placement         string
	IssuedAt          time.Time
}

func ttl(placement string) (time.Duration, bool) {
	switch placement {
	case PlacementSearch, PlacementCompany:
		return SearchTokenTTL, true
	case PlacementEmail:
		return EmailTokenTTL, true
	}
	return 0, false
}

var b64 = base64.RawURLEncoding

func mac(key []byte, payload string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(payload))
	return m.Sum(nil)
}

// SignToken returns the URL-safe token "<payload>.<hmac>". The caller must not sign with an
// empty key: placements are disabled while SPONSORED_CLICK_KEY is unset.
func SignToken(key []byte, c Claims, now time.Time) string {
	payload := fmt.Sprintf("%d:%d:%s:%d", c.CampaignID, c.JobID, c.Placement, now.Unix())
	return b64.EncodeToString([]byte(payload)) + "." + b64.EncodeToString(mac(key, payload))
}

// VerifyToken checks the signature (constant time), the placement and the expiry.
func VerifyToken(key []byte, token string, now time.Time) (Claims, error) {
	if len(key) == 0 {
		return Claims{}, ErrBadToken
	}
	encPayload, encSig, ok := strings.Cut(token, ".")
	if !ok {
		return Claims{}, ErrBadToken
	}
	raw, err := b64.DecodeString(encPayload)
	if err != nil {
		return Claims{}, ErrBadToken
	}
	sig, err := b64.DecodeString(encSig)
	if err != nil || !hmac.Equal(sig, mac(key, string(raw))) {
		return Claims{}, ErrBadToken
	}
	parts := strings.Split(string(raw), ":")
	if len(parts) != 4 {
		return Claims{}, ErrBadToken
	}
	campaign, e1 := strconv.ParseInt(parts[0], 10, 64)
	job, e2 := strconv.ParseInt(parts[1], 10, 64)
	issued, e3 := strconv.ParseInt(parts[3], 10, 64)
	life, known := ttl(parts[2])
	if e1 != nil || e2 != nil || e3 != nil || !known {
		return Claims{}, ErrBadToken
	}
	c := Claims{CampaignID: campaign, JobID: job, Placement: parts[2], IssuedAt: time.Unix(issued, 0)}
	if c.IssuedAt.After(now.Add(time.Minute)) || now.Sub(c.IssuedAt) > life {
		return Claims{}, ErrBadToken
	}
	return c, nil
}

// ClickPath prefixes a token to form the redirect URL a sponsored card links to.
const ClickPath = "/api/v1/sponsored/go/"

// ClickKeyFromEnv reads SPONSORED_CLICK_KEY. Empty means sponsored placements are off: every
// caller must check len(key) > 0 before serving or signing, never sign with an empty key.
func ClickKeyFromEnv() []byte { return []byte(strings.TrimSpace(os.Getenv("SPONSORED_CLICK_KEY"))) }
