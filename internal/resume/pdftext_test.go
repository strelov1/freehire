package resume

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
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

// stubPDFTool points pdftotextBin at a script emitting n bytes of the given UTF-8 filler on
// stdout, so the output-size guard is exercised without hunting for a pathological PDF.
// Restored by t.Cleanup.
func stubPDFTool(t *testing.T, filler string, n int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "spew.sh")
	script := "#!/bin/sh\nyes '" + filler + "' | head -c " + strconv.Itoa(n) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	prev := pdftotextBin
	pdftotextBin = path
	t.Cleanup(func() { pdftotextBin = prev })
}

// A PDF's text layer can be far larger than the file itself, so the extractor's output is
// not bounded by the upload limit: a 472KB file has been observed yielding 12.6MB of text,
// and the whole string then travels to the PII filter and the embedder. Bound what is
// retained instead.
func TestExtractPDFText_BoundsOutputSize(t *testing.T) {
	stubPDFTool(t, "freehire", maxPDFTextBytes+512*1024)

	text, err := ExtractPDFText([]byte("%PDF-1.4 stub"))
	if err != nil {
		t.Fatalf("ExtractPDFText: %v", err)
	}
	if len(text) > maxPDFTextBytes {
		t.Errorf("extracted %d bytes, want at most the %d-byte cap", len(text), maxPDFTextBytes)
	}
	if len(text) == 0 {
		t.Error("extracted nothing; the cap must truncate, not discard")
	}
}

// Truncation lands wherever the cap falls, which for multi-byte text is mid-rune. The text
// goes on to a JSON request body and an embedder, so it must stay valid UTF-8.
func TestExtractPDFText_TruncatesOnARuneBoundary(t *testing.T) {
	// U+2014 is three bytes, so the cap cannot fall on a rune boundary.
	stubPDFTool(t, "———————————————", maxPDFTextBytes+512*1024)

	text, err := ExtractPDFText([]byte("%PDF-1.4 stub"))
	if err != nil {
		t.Fatalf("ExtractPDFText: %v", err)
	}
	if !utf8.ValidString(text) {
		t.Error("truncated text is not valid UTF-8")
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
