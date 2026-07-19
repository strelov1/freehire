package resume

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// requirePDFTool skips a test when pdftotext is not on PATH, mirroring the typst-gated
// renderer test — so `go test ./...` stays green on machines without poppler while the
// extraction is still exercised wherever the production tool is present.
func requirePDFTool(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not on PATH; skipping PDF extraction test")
	}
}

// TestExtractPDFText_CIDIdentityH guards the regression this change fixes: a text-based
// PDF whose fonts are subset CID TrueType with Identity-H encoding + a ToUnicode CMap
// (a Canva export) must yield its selectable text. The pure-Go ledongthuc parser returned
// empty text with no error for these, which surfaced as a bogus "scan or image" rejection.
func TestExtractPDFText_CIDIdentityH(t *testing.T) {
	requirePDFTool(t)

	data, err := os.ReadFile("testdata/canva_cid_identityh.pdf")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	text, err := ExtractPDFText(data)
	if err != nil {
		t.Fatalf("ExtractPDFText: %v", err)
	}
	if strings.TrimSpace(text) == "" {
		t.Fatal("ExtractPDFText returned empty text for a text-based CID/Identity-H PDF")
	}
	// A landmark from the résumé's text layer proves real decoding, not just non-empty noise.
	if !strings.Contains(text, "Frontend Engineer") {
		t.Errorf("extracted text missing expected content; got %d chars", len(text))
	}
}
