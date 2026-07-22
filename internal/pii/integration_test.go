//go:build integration

package pii

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLiveDetectorEndToEnd exercises the real privacy-filter endpoint through the production
// HTTPDetector + Build/Redact/Restore path. Run against a live service:
//
//	PII_FILTER_URL=http://127.0.0.1:8099/detect go test -tags=integration ./internal/pii/
//
// It is skipped when PII_FILTER_URL is unset, so ordinary `go test ./...` never needs it.
func TestLiveDetectorEndToEnd(t *testing.T) {
	url := os.Getenv("PII_FILTER_URL")
	if url == "" {
		t.Skip("PII_FILTER_URL unset; skipping live detector test")
	}
	det := NewHTTPDetector(url, &http.Client{Timeout: 30 * time.Second})

	cv := "Ivan Petrov\nivan@petrov.io | github.com/ivanp\nSenior Go Engineer at RingCentral in London"
	r, err := Build(context.Background(), cv, Contacts{}, det)
	if err != nil {
		t.Fatalf("Build against live detector: %v", err)
	}

	masked := r.Redact(cv)
	for _, leak := range []string{"Ivan Petrov", "ivan@petrov.io", "github.com/ivanp"} {
		if strings.Contains(masked, leak) {
			t.Errorf("live redaction leaked %q:\n%s", leak, masked)
		}
	}
	// Employer must survive (fit analysis needs it).
	if !strings.Contains(masked, "RingCentral") {
		t.Errorf("over-redacted the employer:\n%s", masked)
	}
	// Round-trip restores the original.
	if got := r.Restore(masked); got != cv {
		t.Errorf("round-trip mismatch:\n got %q\nwant %q", got, cv)
	}
	// Contacts recovered from detection.
	if c := r.Contacts(); c.FullName != "Ivan Petrov" || c.Email != "ivan@petrov.io" {
		t.Errorf("contacts = %q/%q, want detected values", c.FullName, c.Email)
	}
}
