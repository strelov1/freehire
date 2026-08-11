package oauth

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"

	"github.com/strelov1/freehire/internal/config"
	"github.com/strelov1/freehire/internal/safehttp"
)

// appleAuthURL, appleTokenURL, and appleJWKSURL are Apple's fixed endpoints.
// appleProvider carries them as fields (rather than inlining the constants at
// each call site) so tests can point them at a stub server.
const (
	appleAuthURL  = "https://appleid.apple.com/auth/authorize"
	appleTokenURL = "https://appleid.apple.com/auth/token"
	appleJWKSURL  = "https://appleid.apple.com/auth/keys"
)

// appleProvider implements Provider for Sign in with Apple. Unlike the OIDC
// providers, it authenticates the token exchange with a self-signed JWT
// (appleClientSecret) instead of a static secret, and it has no userinfo
// endpoint — identity comes only from the token exchange's id_token, whose
// signature must be verified against Apple's JWKS.
type appleProvider struct {
	clientID      string // Apple Services ID
	teamID        string
	keyID         string
	privateKeyPEM string
	redirectURL   string
	authURL       string
	tokenURL      string
	jwksURL       string
}

// NewApple returns the Apple provider ("Sign in with Apple" via the
// authorization-code flow). creds.ClientSecret is unused — Apple authenticates
// with a JWT minted from TeamID/KeyID/PrivateKey instead (see appleClientSecret).
func NewApple(creds config.OAuthCredentials, redirectURL string) Provider {
	return &appleProvider{
		clientID:      creds.ClientID,
		teamID:        creds.TeamID,
		keyID:         creds.KeyID,
		privateKeyPEM: creds.PrivateKey,
		redirectURL:   redirectURL,
		authURL:       appleAuthURL,
		tokenURL:      appleTokenURL,
		jwksURL:       appleJWKSURL,
	}
}

func (p *appleProvider) Name() string { return "apple" }

// AuthCodeURL builds Apple's consent URL. response_mode=form_post is mandatory
// whenever a scope is requested — Apple sends the callback as a POST with a
// form-encoded body instead of GET query parameters.
func (p *appleProvider) AuthCodeURL(state string) string {
	v := url.Values{
		"client_id":     {p.clientID},
		"redirect_uri":  {p.redirectURL},
		"response_type": {"code"},
		"response_mode": {"form_post"},
		"scope":         {"email"},
		"state":         {state},
	}
	return p.authURL + "?" + v.Encode()
}

// FetchIdentity exchanges code for Apple's tokens, using a client secret
// minted fresh for this one call, then verifies the returned id_token's
// signature before trusting its claims. Apple has no userinfo endpoint —
// the id_token is the only source of identity.
func (p *appleProvider) FetchIdentity(ctx context.Context, code string) (Identity, error) {
	ctx = safehttp.GuardedOAuth2Context(ctx, userinfoTimeout)
	client, _ := ctx.Value(oauth2.HTTPClient).(*http.Client)

	secret, err := appleClientSecret(p.teamID, p.clientID, p.keyID, p.privateKeyPEM)
	if err != nil {
		return Identity{}, fmt.Errorf("apple: client secret: %w", err)
	}

	cfg := &oauth2.Config{
		ClientID:     p.clientID,
		ClientSecret: secret,
		Endpoint:     oauth2.Endpoint{TokenURL: p.tokenURL},
		RedirectURL:  p.redirectURL,
	}
	tok, err := cfg.Exchange(ctx, code)
	if err != nil {
		return Identity{}, fmt.Errorf("apple: exchange code: %w", err)
	}

	idToken, ok := tok.Extra("id_token").(string)
	if !ok || idToken == "" {
		return Identity{}, fmt.Errorf("apple: token response has no id_token")
	}

	claims, err := verifyAppleIDToken(ctx, client, p.jwksURL, p.clientID, idToken)
	if err != nil {
		return Identity{}, err
	}
	if claims.Subject == "" {
		return Identity{}, fmt.Errorf("apple: id_token has no sub")
	}
	return Identity{
		ProviderUserID: claims.Subject,
		Email:          claims.Email,
		EmailVerified:  claims.emailVerifiedBool(),
	}, nil
}

// appleClientSecretTTL is how long the self-signed client-secret JWT is valid
// for. It is minted fresh for every token exchange, so this only needs to
// outlive the single round trip to Apple's token endpoint — not the 6-month
// maximum Apple allows.
const appleClientSecretTTL = 5 * time.Minute

// appleClientSecret signs the client-secret JWT Apple requires in place of a
// static client secret: iss is the Team ID, sub is the Services ID (Apple's
// client id), aud is Apple's own issuer, and kid names the signing key.
func appleClientSecret(teamID, servicesID, keyID, privateKeyPEM string) (string, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return "", fmt.Errorf("apple: private key is not valid PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("apple: parse private key: %w", err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("apple: private key is not ECDSA")
	}

	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    teamID,
		Subject:   servicesID,
		Audience:  jwt.ClaimStrings{appleIssuer},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(appleClientSecretTTL)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = keyID
	return token.SignedString(key)
}

// appleIDTokenClaims is the subset of an Apple id_token's claims identity
// resolution needs. EmailVerified is `any` because Apple has been observed to
// send it as either a JSON boolean or the string "true"/"false" depending on
// the flow; emailVerifiedBool normalizes both.
type appleIDTokenClaims struct {
	jwt.RegisteredClaims
	Email         string `json:"email"`
	EmailVerified any    `json:"email_verified"`
}

func (c appleIDTokenClaims) emailVerifiedBool() bool {
	switch v := c.EmailVerified.(type) {
	case bool:
		return v
	case string:
		return v == "true"
	default:
		return false
	}
}

// appleJWK is one key from Apple's published JWKS.
type appleJWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	N   string `json:"n"` // base64url-encoded RSA modulus
	E   string `json:"e"` // base64url-encoded RSA public exponent
}

// applePublicKey decodes a JWK's modulus/exponent into an *rsa.PublicKey.
func (k appleJWK) applePublicKey() (*rsa.PublicKey, error) {
	nb, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("apple: decode JWK modulus: %w", err)
	}
	eb, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("apple: decode JWK exponent: %w", err)
	}
	e := 0
	for _, b := range eb {
		e = e<<8 | int(b)
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: e}, nil
}

// maxJWKSBytes caps Apple's JWKS response — it is a handful of small keys, not
// a multi-MB payload.
const maxJWKSBytes = 1 << 20 // 1 MiB

// fetchAppleJWKS retrieves and decodes Apple's published signing keys.
func fetchAppleJWKS(ctx context.Context, client *http.Client, jwksURL string) ([]appleJWK, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("apple: fetch JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("apple: JWKS: %s", resp.Status)
	}
	var body struct {
		Keys []appleJWK `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJWKSBytes)).Decode(&body); err != nil {
		return nil, fmt.Errorf("apple: decode JWKS: %w", err)
	}
	return body.Keys, nil
}

// appleIssuer is the only issuer Apple ever signs an id_token as.
const appleIssuer = "https://appleid.apple.com"

// verifyAppleIDToken parses idToken and verifies its signature against a key
// fetched from Apple's JWKS matching the token's kid header, and that it was
// issued by Apple for this exact client, before returning its claims. This is
// the one trust boundary in the Apple provider: every other provider proves
// identity via a live, authenticated userinfo call, but Apple hands back only
// this signed token — an unverified signature, audience, or issuer would let
// a forged or misdirected token claim any email.
func verifyAppleIDToken(ctx context.Context, client *http.Client, jwksURL, clientID, idToken string) (appleIDTokenClaims, error) {
	var claims appleIDTokenClaims
	_, err := jwt.ParseWithClaims(idToken, &claims, func(tok *jwt.Token) (any, error) {
		kid, _ := tok.Header["kid"].(string)
		if kid == "" {
			return nil, fmt.Errorf("apple: id_token has no kid")
		}
		keys, err := fetchAppleJWKS(ctx, client, jwksURL)
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			if k.Kid == kid && k.Kty == "RSA" && (k.Use == "" || k.Use == "sig") {
				return k.applePublicKey()
			}
		}
		return nil, fmt.Errorf("apple: no matching RSA signing key for kid %q", kid)
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithAudience(clientID), jwt.WithIssuer(appleIssuer), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithLeeway(30*time.Second))
	if err != nil {
		return appleIDTokenClaims{}, fmt.Errorf("apple: verify id_token: %w", err)
	}
	if claims.Subject == "" {
		return appleIDTokenClaims{}, fmt.Errorf("apple: id_token has no sub")
	}
	return claims, nil
}
