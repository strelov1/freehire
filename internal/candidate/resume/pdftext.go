package resume

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// pdftotextBin is the poppler binary used to extract résumé PDF text, overridable via
// PDFTOTEXT_BIN (default "pdftotext"). A package var so tests can point it at a stub.
var pdftotextBin = cmp.Or(os.Getenv("PDFTOTEXT_BIN"), "pdftotext")

// extractText derives plain text from a stored résumé: a PDF (magic number "%PDF") is
// parsed, anything else is read as UTF-8 text (the pasted-text path). Sniffing the bytes
// keeps the blobstore interface a pure key→bytes abstraction, free of content-type
// plumbing.
func extractText(data []byte) (string, error) {
	if bytes.HasPrefix(data, []byte("%PDF")) {
		return ExtractPDFText(data)
	}
	return string(data), nil
}

// pdfExtractTimeout bounds a single pdftotext run so a pathological PDF can never hang a
// request; extracting a normal résumé is well under a second.
const pdfExtractTimeout = 15 * time.Second

// maxPDFTextBytes bounds the text one extraction retains. A PDF's text layer is not bounded by
// the file size, and the string travels on to structured extraction and the embedder. Two
// megabytes is far past any real résumé, so only a pathological document is truncated.
const maxPDFTextBytes = 2 << 20

// ExtractPDFText extracts plain text from PDF bytes, shared by the résumé store and the upload
// handler (which wraps an error into a 400). It shells out to poppler's pdftotext, which decodes
// CID/Identity-H fonts through their ToUnicode CMap (as Canva produces) where a pure-Go parser
// returned empty text. The bytes go to a temp file (never argv); at most maxPDFTextBytes is read
// from stdout. A non-zero exit is an error; a clean run with no text (an image-only PDF) returns
// ("", nil), which the handler renders as the "scan or image" rejection.
func ExtractPDFText(data []byte) (string, error) {
	dir, err := os.MkdirTemp("", "resume-pdf-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	inPath := filepath.Join(dir, "in.pdf")
	if err := os.WriteFile(inPath, data, 0o600); err != nil { //nolint:gosec // inPath is inside our own MkdirTemp
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), pdfExtractTimeout)
	defer cancel()

	// Only fixed flags and a temp path reach argv: -q silences warnings on slightly
	// malformed but readable PDFs; "-" writes the extracted text to stdout.
	cmd := exec.CommandContext(ctx, pdftotextBin, "-q", "-nopgbrk", inPath, "-") //nolint:gosec // operator-configured binary, fixed flags
	out := cappedBuffer{max: maxPDFTextBytes}
	// stderr only becomes part of an error message, so a few kilobytes is plenty.
	errBuf := cappedBuffer{max: 8 << 10}
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("resume: invalid PDF: %w: %s", err, strings.TrimSpace(errBuf.String()))
	}
	return trimPartialRune(out.String()), nil
}

// cappedBuffer accumulates at most max bytes and discards the rest, so a subprocess writing
// an unbounded amount of output cannot grow this process's heap with it.
//
// It reports the discarded bytes as written on purpose: an error or short count would break the
// child's pipe and turn an enormous document into an extraction error. The timeout bounds the
// child's runtime.
type cappedBuffer struct {
	buf bytes.Buffer
	max int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if room := c.max - c.buf.Len(); room > 0 {
		c.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

func (c *cappedBuffer) String() string { return c.buf.String() }

// trimPartialRune drops a trailing incomplete UTF-8 sequence, which truncating at a byte cap
// leaves behind when the cap falls inside a multi-byte rune; the text goes into JSON, prompts
// and an embedder. A genuine U+FFFD decodes with size 3 and is left alone.
func trimPartialRune(s string) string {
	for s != "" {
		if r, size := utf8.DecodeLastRuneInString(s); r == utf8.RuneError && size <= 1 {
			s = s[:len(s)-1]
			continue
		}
		break
	}
	return s
}
