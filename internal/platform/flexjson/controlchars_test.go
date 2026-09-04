package flexjson

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeControlCharsEscapesInsideStringOnly(t *testing.T) {
	raw := []byte("{\"a\":\"line one\nline two\",\n \"b\":1}")
	got := SanitizeControlChars(raw)
	want := "{\"a\":\"line one\\nline two\",\n \"b\":1}"
	if string(got) != want {
		t.Errorf("SanitizeControlChars = %q, want %q", got, want)
	}
}

func TestSanitizeControlCharsSkipsEscapedBackslash(t *testing.T) {
	// A literal backslash-quote inside the string must not flip inString off early.
	raw := []byte(`{"a":"quote: \" then a real newline` + "\n" + `end"}`)
	got := SanitizeControlChars(raw)
	if strings.Contains(string(got), "\n") {
		t.Errorf("SanitizeControlChars left a raw newline unescaped: %q", got)
	}
}

// A control byte with no two-character escape falls back to \u00XX, so the string's
// contents survive rather than the document being rejected.
func TestSanitizeControlCharsFallsBackToUnicodeEscape(t *testing.T) {
	raw := []byte("{\"a\":\"bell\x07\"}")

	var out struct {
		A string `json:"a"`
	}
	if err := json.Unmarshal(SanitizeControlChars(raw), &out); err != nil {
		t.Fatalf("sanitised document still does not decode: %v", err)
	}
	if out.A != "bell\x07" {
		t.Errorf("decoded %q, want the bell byte preserved", out.A)
	}
}
