package telegram

import "testing"

// A model may emit a literal newline inside a JSON string value (multi-line job
// descriptions are the common case). The strict decoder rejects that, so
// parseExtraction must repair raw control characters before unmarshalling.
func TestParseExtractionRepairsRawNewlines(t *testing.T) {
	raw := "{\"jobs\":[{\"title\":\"Go Dev\",\"description\":\"Line one\nLine two\ttabbed\"}]}"

	got, err := parseExtraction(raw)
	if err != nil {
		t.Fatalf("parseExtraction returned error: %v", err)
	}
	if len(got.Jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(got.Jobs))
	}
	if want := "Line one\nLine two\ttabbed"; got.Jobs[0].Description != want {
		t.Errorf("description = %q, want %q", got.Jobs[0].Description, want)
	}
}

// Already-valid escapes must survive the control-character repair untouched.
// (Markdown-fence stripping is the shared llm client's job, tested there.)
func TestParseExtractionPreservesValidEscapes(t *testing.T) {
	raw := "{\"jobs\":[{\"title\":\"QA\",\"description\":\"a\\nb\"}]}"

	got, err := parseExtraction(raw)
	if err != nil {
		t.Fatalf("parseExtraction returned error: %v", err)
	}
	if want := "a\nb"; got.Jobs[0].Description != want {
		t.Errorf("description = %q, want %q", got.Jobs[0].Description, want)
	}
}
