package sources

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"testing"
)

// russianTrustedRootFingerprint is the SHA-256 of the DER this package embeds, read off
// job.alfabank.ru's own served chain on 2026-09-01. Pinning it here is what makes the
// embedded file auditable: a swapped PEM fails this test rather than silently widening
// what one adapter trusts.
const russianTrustedRootFingerprint = "d26d2d0231b7c39f92cc738512ba54103519e4405d68b5bd703e9788ca8ecf31"

func TestEmbeddedRootIsTheCertWeAudited(t *testing.T) {
	block, _ := pem.Decode(russianTrustedRootCA)
	if block == nil {
		t.Fatal("embedded PEM does not decode")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := sha256Hex(block.Bytes); got != russianTrustedRootFingerprint {
		t.Errorf("fingerprint = %s, want %s — the embedded certificate changed", got, russianTrustedRootFingerprint)
	}
	if !cert.IsCA {
		t.Error("embedded certificate is not a CA")
	}
	if cert.Subject.CommonName != "Russian Trusted Root CA" {
		t.Errorf("CN = %q", cert.Subject.CommonName)
	}
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	const hexdigits = "0123456789abcdef"
	out := make([]byte, 0, 64)
	for _, c := range sum {
		out = append(out, hexdigits[c>>4], hexdigits[c&0x0f])
	}
	return string(out)
}

// The widened trust must reach exactly one client. A default client that also trusted
// this root would extend a state CA's reach to every source we crawl, which is the
// failure this whole shape exists to prevent.
func TestOnlyTheExtraRootClientWidensTrust(t *testing.T) {
	widened := NewRussianTrustedRootClient()
	pool := poolOf(t, widened.httpClient)
	if pool == nil {
		t.Fatal("the widened client must carry an explicit RootCAs pool")
	}

	def := NewClient()
	if p := poolOf(t, def.httpClient); p != nil {
		t.Error("the default client must keep the system trust store untouched")
	}
}

func poolOf(t *testing.T, c *http.Client) *x509.CertPool {
	t.Helper()
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is %T, not *http.Transport", c.Transport)
	}
	if tr.TLSClientConfig == nil {
		return nil
	}
	if tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("verification must stay ON — the point is an extra root, not skipping the check")
	}
	return tr.TLSClientConfig.RootCAs
}
