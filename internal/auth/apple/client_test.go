package apple

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func jsonResponse(status int, body any) *http.Response {
	var b bytes.Buffer
	_ = json.NewEncoder(&b).Encode(body)
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(&b)}
}

func testPrivatePEM(t *testing.T) (string, *ecdsa.PrivateKey) {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), k
}
func b64int(v *big.Int) string { return base64.RawURLEncoding.EncodeToString(v.Bytes()) }
func signIdentity(t *testing.T, key *rsa.PrivateKey, kid, aud, sub, nonce string, expires time.Time) string {
	t.Helper()
	claims := Claims{RegisteredClaims: jwt.RegisteredClaims{Issuer: Issuer, Subject: sub, Audience: jwt.ClaimStrings{aud}, IssuedAt: jwt.NewNumericDate(time.Now().Add(-time.Second)), ExpiresAt: jwt.NewNumericDate(expires)}, Nonce: nonce, Email: "user@example.com", EmailVerified: true}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	raw, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestVerifyExchangeAndRevoke(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	privatePEM, _ := testPrivatePEM(t)
	nonce := "nonce-challenge"
	subject := "apple-subject"
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/keys":
			resp := jsonResponse(200, map[string]any{"keys": []any{map[string]any{"kty": "RSA", "use": "sig", "alg": "RS256", "kid": "kid-1", "n": b64int(rsaKey.N), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(rsaKey.E)).Bytes())}}})
			resp.Header.Set("Cache-Control", "max-age=300")
			return resp, nil
		case "/token":
			_ = r.ParseForm()
			if r.Form.Get("code") == "mismatch" {
				subject = "other"
			}
			return jsonResponse(200, map[string]any{"id_token": signIdentity(t, rsaKey, "kid-1", "me.freehire.mobile", subject, nonce, time.Now().Add(time.Minute)), "refresh_token": "refresh-secret"}), nil
		case "/revoke":
			return jsonResponse(200, map[string]any{}), nil
		default:
			return jsonResponse(404, map[string]any{}), nil
		}
	})}
	client, err := New(Config{TeamID: "TEAM", KeyID: "KEY", PrivateKeyPEM: privatePEM, ClientIDs: []string{"me.freehire.mobile"}, JWKSURL: "https://apple.test/keys", TokenURL: "https://apple.test/token", RevokeURL: "https://apple.test/revoke", HTTPClient: httpClient})
	if err != nil {
		t.Fatal(err)
	}
	raw := signIdentity(t, rsaKey, "kid-1", "me.freehire.mobile", "apple-subject", nonce, time.Now().Add(time.Minute))
	claims, err := client.Verify(context.Background(), raw, nonce)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "apple-subject" {
		t.Fatal(claims.Subject)
	}
	grant, err := client.Exchange(context.Background(), "good", "me.freehire.mobile", nonce, "apple-subject")
	if err != nil {
		t.Fatal(err)
	}
	if grant.RefreshToken != "refresh-secret" {
		t.Fatal("refresh token missing")
	}
	if err = client.Revoke(context.Background(), grant.RefreshToken, grant.ClientID); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Verify(context.Background(), raw, "wrong"); err == nil {
		t.Fatal("wrong nonce accepted")
	}
	if _, err = client.Verify(context.Background(), signIdentity(t, rsaKey, "kid-1", "other-client", "apple-subject", nonce, time.Now().Add(time.Minute)), nonce); err == nil {
		t.Fatal("wrong audience accepted")
	}
	if _, err = client.Verify(context.Background(), signIdentity(t, rsaKey, "kid-1", "me.freehire.mobile", "apple-subject", nonce, time.Now().Add(-time.Minute)), nonce); err == nil {
		t.Fatal("expired token accepted")
	}
	if _, err = client.Exchange(context.Background(), "mismatch", "me.freehire.mobile", nonce, "apple-subject"); err == nil {
		t.Fatal("token response subject mismatch accepted")
	}
}

func TestKeyRingRotationAndAuthentication(t *testing.T) {
	old := make([]byte, 32)
	newKey := make([]byte, 32)
	_, _ = rand.Read(old)
	_, _ = rand.Read(newKey)
	ring, err := NewKeyRing("new", map[string][]byte{"old": old, "new": newKey})
	if err != nil {
		t.Fatal(err)
	}
	aad := GrantAAD("apple", "sub", "client", "row")
	ciphertext, nonce, kid, err := ring.Encrypt([]byte("refresh"), aad)
	if err != nil {
		t.Fatal(err)
	}
	if kid != "new" {
		t.Fatal(kid)
	}
	plain, err := ring.Decrypt(ciphertext, nonce, kid, aad)
	if err != nil || string(plain) != "refresh" {
		t.Fatalf("decrypt=%q err=%v", plain, err)
	}
	ciphertext[0] ^= 1
	if _, err = ring.Decrypt(ciphertext, nonce, kid, aad); err == nil {
		t.Fatal("tamper accepted")
	}
}

func TestKeyForRefreshesExpiredCachedKey(t *testing.T) {
	oldKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	newKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	privatePEM, _ := testPrivatePEM(t)
	fetches := 0
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		fetches++
		return jsonResponse(200, map[string]any{"keys": []any{map[string]any{
			"kty": "RSA", "use": "sig", "alg": "RS256", "kid": "kid-1",
			"n": b64int(newKey.N), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(newKey.E)).Bytes()),
		}}}), nil
	})}
	client, err := New(Config{TeamID: "TEAM", KeyID: "KEY", PrivateKeyPEM: privatePEM,
		ClientIDs: []string{"me.freehire.mobile"}, JWKSURL: "https://apple.test/keys", HTTPClient: httpClient})
	if err != nil {
		t.Fatal(err)
	}
	client.cache = cachedKeys{keys: map[string]*rsa.PublicKey{"kid-1": &oldKey.PublicKey}, expires: time.Now().Add(-time.Minute)}
	got, err := client.keyFor(context.Background(), "kid-1", false)
	if err != nil {
		t.Fatal(err)
	}
	if fetches != 1 || got.N.Cmp(newKey.N) != 0 {
		t.Fatalf("expired cache was reused: fetches=%d", fetches)
	}
}

func TestKeyForRejectsExcessivelyStaleKeyWhenRefreshFails(t *testing.T) {
	oldKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	privatePEM, _ := testPrivatePEM(t)
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("provider unavailable")
	})}
	client, err := New(Config{TeamID: "TEAM", KeyID: "KEY", PrivateKeyPEM: privatePEM,
		ClientIDs: []string{"me.freehire.mobile"}, JWKSURL: "https://apple.test/keys", HTTPClient: httpClient})
	if err != nil {
		t.Fatal(err)
	}
	client.cache = cachedKeys{
		keys:    map[string]*rsa.PublicKey{"kid-1": &oldKey.PublicKey},
		expires: time.Now().Add(-maxJWKSStale - time.Minute),
	}
	if _, err = client.keyFor(context.Background(), "kid-1", false); err == nil {
		t.Fatal("excessively stale key accepted")
	}
}
