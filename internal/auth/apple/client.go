// Package apple implements the native Sign in with Apple trust boundary. It
// verifies nonce-bound identity tokens, exchanges authorization codes on the
// server, encrypts reusable grants, and revokes them without exposing tokens.
package apple

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/sync/singleflight"
)

const (
	Issuer           = "https://appleid.apple.com"
	DefaultTokenURL  = "https://appleid.apple.com/auth/token"
	DefaultJWKSURL   = "https://appleid.apple.com/auth/keys"
	DefaultRevokeURL = "https://appleid.apple.com/auth/revoke"
	maxBody          = 1 << 20
	maxJWKSStale     = 24 * time.Hour
)

var (
	ErrInvalidToken = errors.New("apple: invalid identity token")
	ErrExchange     = errors.New("apple: authorization code exchange failed")
	ErrRevoke       = errors.New("apple: token revocation failed")
	ErrPermanent    = errors.New("apple: permanent provider configuration error")
	ErrGrantDecrypt = errors.New("apple: grant decryption failed")
)

type Config struct {
	TeamID, KeyID, PrivateKeyPEM string
	ClientIDs                    []string
	TokenURL, JWKSURL, RevokeURL string
	HTTPClient                   *http.Client
	Leeway                       time.Duration
}

type Claims struct {
	jwt.RegisteredClaims
	Nonce         string `json:"nonce"`
	Email         string `json:"email"`
	EmailVerified any    `json:"email_verified"`
}

func (c Claims) VerifiedEmail() (string, bool) {
	verified := false
	switch v := c.EmailVerified.(type) {
	case bool:
		verified = v
	case string:
		verified = v == "true"
	}
	return c.Email, verified && c.Email != ""
}

type Grant struct {
	Subject, ClientID, RefreshToken string
	Claims                          Claims
}

type jwk struct{ Kty, Use, Kid, Alg, N, E string }
type cachedKeys struct {
	keys    map[string]*rsa.PublicKey
	expires time.Time
}

type Client struct {
	cfg     Config
	key     *ecdsa.PrivateKey
	mu      sync.RWMutex
	cache   cachedKeys
	refresh singleflight.Group
	now     func() time.Time
}

func New(cfg Config) (*Client, error) {
	if cfg.TeamID == "" || cfg.KeyID == "" || cfg.PrivateKeyPEM == "" || len(cfg.ClientIDs) == 0 {
		return nil, errors.New("apple: incomplete configuration")
	}
	block, _ := pem.Decode([]byte(cfg.PrivateKeyPEM))
	if block == nil {
		return nil, errors.New("apple: private key is not PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("apple: parse private key: %w", err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("apple: private key is not ECDSA")
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = DefaultTokenURL
	}
	if cfg.JWKSURL == "" {
		cfg.JWKSURL = DefaultJWKSURL
	}
	if cfg.RevokeURL == "" {
		cfg.RevokeURL = DefaultRevokeURL
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	if cfg.Leeway <= 0 {
		cfg.Leeway = 30 * time.Second
	}
	seen := map[string]bool{}
	for _, id := range cfg.ClientIDs {
		if strings.TrimSpace(id) == "" || seen[id] {
			return nil, errors.New("apple: invalid client id allowlist")
		}
		seen[id] = true
	}
	return &Client{cfg: cfg, key: key, now: time.Now}, nil
}

func (c *Client) Enabled() bool       { return c != nil }
func (c *Client) ClientIDs() []string { return append([]string(nil), c.cfg.ClientIDs...) }

func (c *Client) clientAllowed(aud string) bool {
	for _, id := range c.cfg.ClientIDs {
		if id == aud {
			return true
		}
	}
	return false
}

func (c *Client) ClientIDForClaims(claims Claims) (string, bool) {
	for _, aud := range claims.Audience {
		if c.clientAllowed(aud) {
			return aud, true
		}
	}
	return "", false
}

func parseMaxAge(header string) time.Duration {
	for _, item := range strings.Split(header, ",") {
		item = strings.TrimSpace(item)
		if strings.HasPrefix(item, "max-age=") {
			if n, err := strconv.Atoi(strings.TrimPrefix(item, "max-age=")); err == nil && n > 0 && n <= 86400 {
				return time.Duration(n) * time.Second
			}
		}
	}
	return time.Hour
}

func (c *Client) fetchKeys(ctx context.Context) (cachedKeys, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.JWKSURL, nil)
	if err != nil {
		return cachedKeys{}, err
	}
	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return cachedKeys{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return cachedKeys{}, fmt.Errorf("apple: jwks status %d", resp.StatusCode)
	}
	var body struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&body); err != nil {
		return cachedKeys{}, err
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range body.Keys {
		if k.Kid == "" || k.Kty != "RSA" || (k.Use != "" && k.Use != "sig") || (k.Alg != "" && k.Alg != "RS256") {
			continue
		}
		nb, e1 := base64.RawURLEncoding.DecodeString(k.N)
		eb, e2 := base64.RawURLEncoding.DecodeString(k.E)
		if e1 != nil || e2 != nil || len(nb) == 0 || len(eb) == 0 {
			continue
		}
		e := 0
		for _, b := range eb {
			e = e<<8 | int(b)
		}
		if e < 3 {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: e}
	}
	if len(keys) == 0 {
		return cachedKeys{}, errors.New("apple: jwks contains no usable keys")
	}
	return cachedKeys{keys: keys, expires: c.now().Add(parseMaxAge(resp.Header.Get("Cache-Control")))}, nil
}

func (c *Client) keyFor(ctx context.Context, kid string, force bool) (*rsa.PublicKey, error) {
	if !force {
		c.mu.RLock()
		cached, ok, fresh := c.cache.keys[kid], c.cache.keys != nil, c.cache.expires.After(c.now())
		c.mu.RUnlock()
		if cached != nil && fresh {
			return cached, nil
		}
		if ok && fresh {
			return nil, fmt.Errorf("apple: unknown kid")
		}
	}
	v, err, _ := c.refresh.Do("jwks", func() (any, error) { return c.fetchKeys(ctx) })
	if err != nil {
		c.mu.RLock()
		stale, staleUntil := c.cache.keys[kid], c.cache.expires.Add(maxJWKSStale)
		c.mu.RUnlock()
		if stale != nil && c.now().Before(staleUntil) {
			return stale, nil
		}
		return nil, err
	}
	cache := v.(cachedKeys)
	c.mu.Lock()
	c.cache = cache
	c.mu.Unlock()
	key := cache.keys[kid]
	if key == nil {
		return nil, errors.New("apple: unknown kid")
	}
	return key, nil
}

func (c *Client) Verify(ctx context.Context, raw, expectedNonce string) (Claims, error) {
	if raw == "" || expectedNonce == "" {
		return Claims{}, ErrInvalidToken
	}
	var unverified Claims
	tok, _, err := jwt.NewParser().ParseUnverified(raw, &unverified)
	if err != nil || tok.Method.Alg() != jwt.SigningMethodRS256.Alg() {
		return Claims{}, ErrInvalidToken
	}
	kid, ok := tok.Header["kid"].(string)
	if !ok || kid == "" {
		return Claims{}, ErrInvalidToken
	}
	key, err := c.keyFor(ctx, kid, false)
	if err != nil {
		key, err = c.keyFor(ctx, kid, true)
	}
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	var claims Claims
	parsed, err := jwt.ParseWithClaims(raw, &claims, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodRS256 {
			return nil, ErrInvalidToken
		}
		if t.Header["kid"] != kid {
			return nil, ErrInvalidToken
		}
		return key, nil
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer(Issuer), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithLeeway(c.cfg.Leeway))
	if err != nil || !parsed.Valid || claims.Subject == "" || claims.Nonce != expectedNonce || len(claims.Audience) == 0 {
		return Claims{}, ErrInvalidToken
	}
	allowed := false
	for _, aud := range claims.Audience {
		if c.clientAllowed(aud) {
			allowed = true
			break
		}
	}
	if !allowed || claims.IssuedAt == nil || claims.IssuedAt.After(c.now().Add(c.cfg.Leeway)) {
		return Claims{}, ErrInvalidToken
	}
	return claims, nil
}

func (c *Client) clientSecret(clientID string) (string, error) {
	if !c.clientAllowed(clientID) {
		return "", errors.New("apple: client id not allowed")
	}
	now := c.now()
	claims := jwt.RegisteredClaims{Issuer: c.cfg.TeamID, Subject: clientID, Audience: jwt.ClaimStrings{Issuer}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute))}
	t := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	t.Header["kid"] = c.cfg.KeyID
	return t.SignedString(c.key)
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	Error        string `json:"error"`
}

func (c *Client) postForm(ctx context.Context, endpoint string, form url.Values, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if out != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(out); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

func (c *Client) Exchange(ctx context.Context, code, clientID, expectedNonce, submittedSubject string) (Grant, error) {
	if code == "" || submittedSubject == "" {
		return Grant{}, ErrExchange
	}
	secret, err := c.clientSecret(clientID)
	if err != nil {
		return Grant{}, err
	}
	var tr tokenResponse
	status, err := c.postForm(ctx, c.cfg.TokenURL, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {clientID}, "client_secret": {secret}}, &tr)
	if err != nil || status != http.StatusOK || tr.Error != "" || tr.IDToken == "" || tr.RefreshToken == "" {
		return Grant{}, ErrExchange
	}
	claims, err := c.Verify(ctx, tr.IDToken, expectedNonce)
	if err != nil || claims.Subject != submittedSubject {
		return Grant{}, ErrExchange
	}
	audOK := false
	for _, aud := range claims.Audience {
		if aud == clientID {
			audOK = true
		}
	}
	if !audOK {
		return Grant{}, ErrExchange
	}
	return Grant{Subject: claims.Subject, ClientID: clientID, RefreshToken: tr.RefreshToken, Claims: claims}, nil
}

func (c *Client) Revoke(ctx context.Context, refreshToken, clientID string) error {
	secret, err := c.clientSecret(clientID)
	if err != nil {
		return err
	}
	form := url.Values{"token": {refreshToken}, "token_type_hint": {"refresh_token"}, "client_id": {clientID}, "client_secret": {secret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.RevokeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	var payload struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&payload)
	if resp.StatusCode == http.StatusBadRequest && payload.Error == "invalid_grant" {
		return nil
	}
	if resp.StatusCode == http.StatusBadRequest && (payload.Error == "invalid_client" || payload.Error == "unauthorized_client") {
		return fmt.Errorf("%w: %s", ErrPermanent, payload.Error)
	}
	return fmt.Errorf("%w: status %d", ErrRevoke, resp.StatusCode)
}

type KeyRing struct {
	active string
	keys   map[string][]byte
}

func NewKeyRing(active string, keys map[string][]byte) (*KeyRing, error) {
	if active == "" || len(keys) == 0 {
		return nil, errors.New("apple: empty grant key ring")
	}
	copied := map[string][]byte{}
	for id, k := range keys {
		if id == "" || len(k) != 32 {
			return nil, errors.New("apple: grant keys must be named AES-256 keys")
		}
		copied[id] = append([]byte(nil), k...)
	}
	if copied[active] == nil {
		return nil, errors.New("apple: active grant key missing")
	}
	return &KeyRing{active: active, keys: copied}, nil
}
func (r *KeyRing) Encrypt(plain, aad []byte) (ciphertext, nonce []byte, keyID string, err error) {
	block, _ := aes.NewCipher(r.keys[r.active])
	gcm, _ := cipher.NewGCM(block)
	nonce = make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return
	}
	ciphertext = gcm.Seal(nil, nonce, plain, aad)
	keyID = r.active
	return
}
func (r *KeyRing) Decrypt(ciphertext, nonce []byte, keyID string, aad []byte) ([]byte, error) {
	key := r.keys[keyID]
	if key == nil {
		return nil, fmt.Errorf("%w: unknown key", ErrGrantDecrypt)
	}
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("%w: authentication failed", ErrGrantDecrypt)
	}
	return plain, nil
}
func GrantAAD(provider, subject, clientID, rowID string) []byte {
	sum := sha256.Sum256([]byte("v1\x00" + provider + "\x00" + subject + "\x00" + clientID + "\x00" + rowID))
	return sum[:]
}
