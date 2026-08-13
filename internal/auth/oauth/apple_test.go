package oauth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// testAppleRSAKey generates a throwaway RSA key pair and serves it as Apple's
// JWKS would, under the given kid.
func testAppleRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	return key
}

func stubAppleJWKS(t *testing.T, kid string, key *rsa.PrivateKey) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/keys", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{
				{
					"kty": "RSA",
					"kid": kid,
					"use": "sig",
					"alg": "RS256",
					"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
					"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
				},
			},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func signAppleIDToken(t *testing.T, key *rsa.PrivateKey, kid string, claims appleIDTokenClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign id_token: %v", err)
	}
	return signed
}

// testAppleKeyPEM generates a throwaway ES256 private key PEM for tests — never
// the real Apple .p8 key.
func testAppleKeyPEM(t *testing.T) (string, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: der}
	return string(pem.EncodeToMemory(block)), key
}

func TestAppleClientSecret_ClaimsAndHeader(t *testing.T) {
	pemKey, key := testAppleKeyPEM(t)

	secret, err := appleClientSecret("TEAM123456", "me.freehire.web", "KEY1234567", pemKey)
	if err != nil {
		t.Fatalf("appleClientSecret: %v", err)
	}

	claims := jwt.RegisteredClaims{}
	tok, err := jwt.ParseWithClaims(secret, &claims, func(tok *jwt.Token) (any, error) {
		return &key.PublicKey, nil
	}, jwt.WithValidMethods([]string{"ES256"}))
	if err != nil {
		t.Fatalf("parse minted secret: %v", err)
	}
	if !tok.Valid {
		t.Fatal("minted secret did not validate against its own key")
	}
	if kid, _ := tok.Header["kid"].(string); kid != "KEY1234567" {
		t.Errorf("kid header = %q, want KEY1234567", kid)
	}
	if claims.Issuer != "TEAM123456" {
		t.Errorf("iss = %q, want TEAM123456", claims.Issuer)
	}
	if claims.Subject != "me.freehire.web" {
		t.Errorf("sub = %q, want me.freehire.web", claims.Subject)
	}
	if len(claims.Audience) != 1 || claims.Audience[0] != "https://appleid.apple.com" {
		t.Errorf("aud = %v, want [https://appleid.apple.com]", claims.Audience)
	}
	if claims.ExpiresAt == nil || claims.IssuedAt == nil {
		t.Fatal("missing iat/exp")
	}
	if ttl := claims.ExpiresAt.Sub(claims.IssuedAt.Time); ttl != appleClientSecretTTL {
		t.Errorf("ttl = %v, want %v", ttl, appleClientSecretTTL)
	}
}

func TestAppleClientSecret_InvalidPEM(t *testing.T) {
	if _, err := appleClientSecret("TEAM123456", "me.freehire.web", "KEY1234567", "not a pem"); err == nil {
		t.Error("want error for invalid PEM, got nil")
	}
}

func TestApple_AuthCodeURL(t *testing.T) {
	p := &appleProvider{
		clientID:    "me.freehire.web",
		redirectURL: "https://freehire.me/api/v1/auth/oauth/apple/callback",
		authURL:     "https://appleid.apple.com/auth/authorize",
	}
	u := p.AuthCodeURL("the-state")

	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatalf("parse AuthCodeURL: %v", err)
	}
	q := parsed.Query()
	for key, want := range map[string]string{
		"client_id":     "me.freehire.web",
		"redirect_uri":  "https://freehire.me/api/v1/auth/oauth/apple/callback",
		"response_type": "code",
		"response_mode": "form_post",
		"scope":         "email",
		"state":         "the-state",
	} {
		if got := q.Get(key); got != want {
			t.Errorf("query[%q] = %q, want %q", key, got, want)
		}
	}
}

func TestVerifyAppleIDToken_Valid(t *testing.T) {
	key := testAppleRSAKey(t)
	srv := stubAppleJWKS(t, "the-kid", key)

	now := time.Now()
	idToken := signAppleIDToken(t, key, "the-kid", appleIDTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "apple-sub-1",
			Audience:  jwt.ClaimStrings{"me.freehire.web"},
			Issuer:    "https://appleid.apple.com",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
		Email:         "user@example.com",
		EmailVerified: true,
	})

	claims, err := verifyAppleIDToken(context.Background(), srv.Client(), srv.URL+"/auth/keys", "me.freehire.web", idToken)
	if err != nil {
		t.Fatalf("verifyAppleIDToken: %v", err)
	}
	if claims.Subject != "apple-sub-1" {
		t.Errorf("sub = %q, want apple-sub-1", claims.Subject)
	}
	if claims.Email != "user@example.com" {
		t.Errorf("email = %q, want user@example.com", claims.Email)
	}
	if !claims.emailVerifiedBool() {
		t.Error("email_verified = false, want true")
	}
}

func TestVerifyAppleIDToken_EmailVerifiedAsString(t *testing.T) {
	// Apple has been known to send email_verified as the string "true" rather
	// than the boolean true, depending on the flow.
	key := testAppleRSAKey(t)
	srv := stubAppleJWKS(t, "the-kid", key)
	now := time.Now()
	idToken := signAppleIDToken(t, key, "the-kid", appleIDTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "apple-sub-2",
			Audience:  jwt.ClaimStrings{"me.freehire.web"},
			Issuer:    "https://appleid.apple.com",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
		Email:         "user2@example.com",
		EmailVerified: "true",
	})

	claims, err := verifyAppleIDToken(context.Background(), srv.Client(), srv.URL+"/auth/keys", "me.freehire.web", idToken)
	if err != nil {
		t.Fatalf("verifyAppleIDToken: %v", err)
	}
	if !claims.emailVerifiedBool() {
		t.Error("email_verified = false, want true (from string \"true\")")
	}
}

func TestVerifyAppleIDToken_WrongSigningKeyRejected(t *testing.T) {
	realKey := testAppleRSAKey(t)
	wrongKey := testAppleRSAKey(t)
	srv := stubAppleJWKS(t, "the-kid", realKey)

	now := time.Now()
	idToken := signAppleIDToken(t, wrongKey, "the-kid", appleIDTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "apple-sub-3",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
		Email:         "user3@example.com",
		EmailVerified: true,
	})

	if _, err := verifyAppleIDToken(context.Background(), srv.Client(), srv.URL+"/auth/keys", "me.freehire.web", idToken); err == nil {
		t.Error("want error for id_token signed by a key not in the JWKS, got nil")
	}
}

func TestVerifyAppleIDToken_UnknownKidRejected(t *testing.T) {
	key := testAppleRSAKey(t)
	srv := stubAppleJWKS(t, "the-real-kid", key)

	now := time.Now()
	idToken := signAppleIDToken(t, key, "some-other-kid", appleIDTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "apple-sub-4",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	})

	if _, err := verifyAppleIDToken(context.Background(), srv.Client(), srv.URL+"/auth/keys", "me.freehire.web", idToken); err == nil {
		t.Error("want error for id_token whose kid is not in the JWKS, got nil")
	}
}

func TestVerifyAppleIDToken_WrongAudienceRejected(t *testing.T) {
	key := testAppleRSAKey(t)
	srv := stubAppleJWKS(t, "the-kid", key)

	now := time.Now()
	idToken := signAppleIDToken(t, key, "the-kid", appleIDTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "apple-sub-5",
			Audience:  jwt.ClaimStrings{"someone-elses-services-id"},
			Issuer:    "https://appleid.apple.com",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
		Email:         "user5@example.com",
		EmailVerified: true,
	})

	if _, err := verifyAppleIDToken(context.Background(), srv.Client(), srv.URL+"/auth/keys", "me.freehire.web", idToken); err == nil {
		t.Error("want error for id_token issued for a different audience, got nil")
	}
}

func TestVerifyAppleIDToken_WrongIssuerRejected(t *testing.T) {
	key := testAppleRSAKey(t)
	srv := stubAppleJWKS(t, "the-kid", key)

	now := time.Now()
	idToken := signAppleIDToken(t, key, "the-kid", appleIDTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "apple-sub-6",
			Audience:  jwt.ClaimStrings{"me.freehire.web"},
			Issuer:    "https://not-apple.example.com",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
		Email:         "user6@example.com",
		EmailVerified: true,
	})

	if _, err := verifyAppleIDToken(context.Background(), srv.Client(), srv.URL+"/auth/keys", "me.freehire.web", idToken); err == nil {
		t.Error("want error for id_token issued by a non-Apple issuer, got nil")
	}
}

// stubApple serves Apple's token-exchange and JWKS endpoints together, so
// FetchIdentity can run its whole round trip against one stub server. The
// token endpoint always hands back an id_token signed by key under kid.
func stubApple(t *testing.T, key *rsa.PrivateKey, kid string, idTokenClaims appleIDTokenClaims) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/keys", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{
				{
					"kty": "RSA",
					"kid": kid,
					"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
					"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
				},
			},
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "stub-access-token",
			"token_type":   "Bearer",
			"id_token":     signAppleIDToken(t, key, kid, idTokenClaims),
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func appleForTest(t *testing.T, srv *httptest.Server) *appleProvider {
	pemKey, _ := testAppleKeyPEM(t)
	return &appleProvider{
		clientID:      "me.freehire.web",
		teamID:        "TEAM123456",
		keyID:         "KEY1234567",
		privateKeyPEM: pemKey,
		redirectURL:   "http://app/callback",
		tokenURL:      srv.URL + "/token",
		jwksURL:       srv.URL + "/auth/keys",
	}
}

func TestApple_FetchIdentity(t *testing.T) {
	key := testAppleRSAKey(t)
	now := time.Now()
	srv := stubApple(t, key, "the-kid", appleIDTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "apple-sub-1",
			Audience:  jwt.ClaimStrings{"me.freehire.web"},
			Issuer:    "https://appleid.apple.com",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
		Email:         "user@example.com",
		EmailVerified: true,
	})

	got, err := appleForTest(t, srv).FetchIdentity(testClientCtx(srv), "code")
	if err != nil {
		t.Fatalf("FetchIdentity: %v", err)
	}
	want := Identity{ProviderUserID: "apple-sub-1", Email: "user@example.com", EmailVerified: true}
	if got != want {
		t.Errorf("identity = %+v, want %+v", got, want)
	}
}

func TestApple_FetchIdentityMissingSub(t *testing.T) {
	key := testAppleRSAKey(t)
	now := time.Now()
	srv := stubApple(t, key, "the-kid", appleIDTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Audience:  jwt.ClaimStrings{"me.freehire.web"},
			Issuer:    "https://appleid.apple.com",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
		Email: "user@example.com",
	})

	if _, err := appleForTest(t, srv).FetchIdentity(testClientCtx(srv), "code"); err == nil {
		t.Error("want error for id_token without sub, got nil")
	}
}
