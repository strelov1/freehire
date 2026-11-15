package sources

import (
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"log"
	"net/http"
	"time"

	"github.com/strelov1/freehire/internal/platform/safehttp"
)

// russianTrustedRootCA is the "Russian Trusted Root CA" certificate (Ministry of Digital
// Development), read off job.alfabank.ru's own served chain rather than downloaded from a third
// party. extraroot_test.go pins its SHA-256, so replacing this file is a visible, failing edit.
//
// Alfa-Bank's careers API moved to a certificate this CA issued, which no standard trust store
// carries, so the crawl failed at the TLS handshake. ptsecurity.com, hr.tochka.com and
// www.tbank.ru serve certificates chaining to the same root.
//
//go:embed russian_trusted_root_ca.pem
var russianTrustedRootCA []byte

// NewRussianTrustedRootClient builds an ingest client that trusts the system roots PLUS
// the certificate above, and is handed only to the adapters whose hosts serve a certificate
// this CA issued (alfabank, ptsecurity, tochka and tbank, in All).
//
// It ADDS a root; it does not skip verification. InsecureSkipVerify would accept any
// certificate from any host this client touches, which turns a narrow "we accept this
// state CA for one Russian job board" into "this client can be MITM'd anywhere". The pool
// starts from the system store rather than replacing it, so the client keeps verifying
// every ordinary host normally and the widening is strictly additive.
//
// The scope is the load-bearing part. A state CA can issue a certificate for any name, so
// trusting one everywhere would let its holder impersonate every source we crawl. One
// client, four adapters, four hosts' worth of exposure — and the crawl sends no
// credentials and reads only public listings, so the worst case is a forged job feed
// rather than a leak.
//
// A system pool that cannot be read (unusual, but possible on a stripped image) degrades
// to a pool holding only this root: those adapters still work, and any other host this
// client touched would fail closed rather than silently unverified. Nothing else uses it.
func NewRussianTrustedRootClient() *Client {
	return newRussianTrustedRootClient(15 * time.Second)
}

// newRussianTrustedRootClient is NewRussianTrustedRootClient with the buffered-fetch timeout
// as a parameter, for a host whose responses are slow (tbank's sitemaps).
func newRussianTrustedRootClient(timeout time.Duration) *Client {
	pool, err := x509.SystemCertPool()
	if err != nil {
		log.Printf("sources: system cert pool unavailable (%v); the Russian-root client trusts only its embedded root", err)
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(russianTrustedRootCA) {
		// A PEM that does not parse leaves the client with the system store alone: these adapters
		// fail at the handshake, loudly, and no other source is affected.
		log.Printf("sources: embedded Russian Trusted Root CA did not parse; alfabank, ptsecurity and tochka will keep failing their TLS handshakes")
	}

	c := NewClient()
	c.httpClient = clientWithRoots(timeout, pool)
	c.streamClient = clientWithRoots(streamTimeout, pool)
	return c
}

// clientWithRoots is safehttp's guarded client with an explicit trust pool. The transport
// keeps the SSRF guard, the timeouts and the proxy-less dialer it was built with; only
// TLSClientConfig is added.
func clientWithRoots(timeout time.Duration, pool *x509.CertPool) *http.Client {
	t := safehttp.NewTransport(5 * time.Second)
	t.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	return &http.Client{Timeout: timeout, Transport: t}
}
