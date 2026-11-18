package ojcp

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/gowebpki/jcs"
)

// signatureLifetime is how long a manifest signature stays valid (the spec's recommended
// 24 hours); it bounds how long a signature can outlive a key rotation.
const signatureLifetime = 24 * time.Hour

// Signature is the manifest's `signature` member (spec § Manifest Signing).
type Signature struct {
	Alg       string `json:"alg"`
	Kid       string `json:"kid"`
	Iss       string `json:"iss"`
	SignedAt  string `json:"signed_at"`
	ExpiresAt string `json:"expires_at"`
	// Value is the JWS signature: raw R||S of an ES256 signature, base64url without padding
	// (RFC 7518 §3.4). The spec's example shows DER, which JOSE libraries do not accept.
	Value string `json:"value"`
}

// JWKSet is the document served at /.well-known/ojcp-keys.json.
type JWKSet struct {
	Keys []JWK `json:"keys"`
}

// JWK is one public P-256 key, carrying the kid, kty and alg the spec requires of each.
type JWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
}

// Signer signs this deployment's manifest and publishes the key that verifies it.
type Signer struct {
	key *ecdsa.PrivateKey
	jwk JWK
	iss string
}

// NewSigner reads a PEM-encoded P-256 private key (PKCS#8 or SEC 1). iss is the domain the
// manifest is served from, which an agent compares against the one it fetched it from.
//
// The kid is the key's RFC 7638 thumbprint rather than a configured name, so replacing the
// key file can never publish a new key under an old kid.
func NewSigner(pemKey []byte, iss string) (*Signer, error) {
	block, _ := pem.Decode(pemKey)
	if block == nil {
		return nil, errors.New("ojcp signing key: no PEM block")
	}
	key, err := parseECKey(block)
	if err != nil {
		return nil, err
	}
	if key.Curve != elliptic.P256() {
		return nil, fmt.Errorf("ojcp signing key: curve %s, want P-256 for ES256", key.Curve.Params().Name)
	}
	pub, err := key.PublicKey.ECDH()
	if err != nil {
		return nil, fmt.Errorf("ojcp signing key: %w", err)
	}
	// An uncompressed point is 0x04 || X || Y, each coordinate 32 bytes on P-256.
	point := pub.Bytes()
	jwk := JWK{
		Kty: "EC",
		Crv: "P-256",
		X:   base64.RawURLEncoding.EncodeToString(point[1:33]),
		Y:   base64.RawURLEncoding.EncodeToString(point[33:]),
		Alg: "ES256",
		Use: "sig",
	}
	jwk.Kid = thumbprint(jwk)
	return &Signer{key: key, jwk: jwk, iss: iss}, nil
}

func parseECKey(block *pem.Block) (*ecdsa.PrivateKey, error) {
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("ojcp signing key: %w", err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("ojcp signing key: %T, want an ECDSA key", parsed)
	}
	return key, nil
}

// thumbprint is the RFC 7638 JWK thumbprint: SHA-256 over the required members of an EC
// key, in lexicographic order, with no whitespace.
func thumbprint(k JWK) string {
	members := fmt.Sprintf(`{"crv":%q,"kty":%q,"x":%q,"y":%q}`, k.Crv, k.Kty, k.X, k.Y)
	sum := sha256.Sum256([]byte(members))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// KeySet is the JWK Set agents verify the manifest against.
func (s *Signer) KeySet() JWKSet {
	return JWKSet{Keys: []JWK{s.jwk}}
}

// Sign returns m carrying a signature made at now. The signature covers the JCS
// canonicalisation (RFC 8785) of the manifest with its `signature` member removed, which
// is what spec § Canonicalization makes both sides compute.
//
// It panics only on a manifest that cannot be marshalled or a failing system RNG — both
// programming or platform faults no request could recover from.
func (s *Signer) Sign(m Manifest, now time.Time) Manifest {
	m.Signature = nil
	body, err := json.Marshal(m)
	if err != nil {
		panic(fmt.Sprintf("ojcp: marshal manifest: %v", err))
	}
	canonical, err := jcs.Transform(body)
	if err != nil {
		panic(fmt.Sprintf("ojcp: canonicalise manifest: %v", err))
	}
	digest := sha256.Sum256(canonical)
	r, sv, err := ecdsa.Sign(rand.Reader, s.key, digest[:])
	if err != nil {
		panic(fmt.Sprintf("ojcp: sign manifest: %v", err))
	}
	raw := make([]byte, 64)
	r.FillBytes(raw[:32])
	sv.FillBytes(raw[32:])

	now = now.UTC().Truncate(time.Second)
	m.Signature = &Signature{
		Alg:       "ES256",
		Kid:       s.jwk.Kid,
		Iss:       s.iss,
		SignedAt:  now.Format(time.RFC3339),
		ExpiresAt: now.Add(signatureLifetime).Format(time.RFC3339),
		Value:     base64.RawURLEncoding.EncodeToString(raw),
	}
	return m
}
