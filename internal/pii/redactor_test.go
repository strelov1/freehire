package pii

import (
	"context"
	"strings"
	"testing"
)

// nameDetector is a fake Detector that reports each configured name as a NAME span.
type nameDetector struct{ names []string }

func (f nameDetector) Detect(_ context.Context, text string) ([]Span, error) {
	var spans []Span
	for _, n := range f.names {
		if i := strings.Index(text, n); i >= 0 {
			spans = append(spans, Span{i, i + len(n), KindName})
		}
	}
	return spans, nil
}

func mustBuild(t *testing.T, text string, known Contacts, d Detector) *Redactor {
	t.Helper()
	r, err := Build(context.Background(), text, known, d)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return r
}

func TestRedactMasksAllPII(t *testing.T) {
	cv := "Ada Lovelace\nada.lovelace@example.com | github.com/adalovelace\nSenior Engineer at RingCentral in London"
	r := mustBuild(t, cv, Contacts{}, nameDetector{names: []string{"Ada Lovelace"}})
	masked := r.Redact(cv)

	for _, leak := range []string{"Ada Lovelace", "ada.lovelace@example.com", "github.com/adalovelace"} {
		if strings.Contains(masked, leak) {
			t.Errorf("masked text still contains PII %q:\n%s", leak, masked)
		}
	}
	// Non-PII context must survive.
	for _, keep := range []string{"RingCentral", "London", "Senior Engineer"} {
		if !strings.Contains(masked, keep) {
			t.Errorf("masked text dropped non-PII %q:\n%s", keep, masked)
		}
	}
}

func TestRestoreRoundTrip(t *testing.T) {
	cv := "Ada Lovelace — ada.lovelace@example.com — github.com/adalovelace"
	r := mustBuild(t, cv, Contacts{}, nameDetector{names: []string{"Ada Lovelace"}})
	if got := r.Restore(r.Redact(cv)); got != cv {
		t.Fatalf("round-trip mismatch:\n got %q\nwant %q", got, cv)
	}
}

func TestDistinctValuesGetDistinctPlaceholders(t *testing.T) {
	text := "primary a@x.com secondary b@y.com"
	r := mustBuild(t, text, Contacts{}, nameDetector{})
	masked := r.Redact(text)
	if strings.Contains(masked, "a@x.com") || strings.Contains(masked, "b@y.com") {
		t.Fatalf("emails not masked: %s", masked)
	}
	// Two distinct emails -> two distinct placeholders that restore independently.
	if r.Restore(masked) != text {
		t.Fatalf("distinct emails did not restore: %q", r.Restore(masked))
	}
	if strings.Count(masked, "[REDACTED_EMAIL_1]") != 1 || strings.Count(masked, "[REDACTED_EMAIL_2]") != 1 {
		t.Fatalf("expected two numbered email placeholders, got: %s", masked)
	}
}

func TestKnownContactsMaskedInOtherText(t *testing.T) {
	// The CV text the Redactor is built from, plus a separate structured-JSON blob that
	// carries the same contacts — both must mask with the SAME redactor (matchanalysis case).
	cv := "Ada Lovelace works remotely"
	structured := `{"full_name":"Ada Lovelace","email":"ada.lovelace@example.com"}`
	known := Contacts{FullName: "Ada Lovelace", Email: "ada.lovelace@example.com"}
	r := mustBuild(t, cv, known, nameDetector{names: []string{"Ada Lovelace"}})
	masked := r.Redact(structured)
	if strings.Contains(masked, "Ada Lovelace") || strings.Contains(masked, "ada.lovelace@example.com") {
		t.Fatalf("known contacts leaked in structured blob: %s", masked)
	}
}

// spansDetector returns a fixed span set, to reproduce messy real-CV detections.
type spansDetector struct{ spans []Span }

func (d spansDetector) Detect(_ context.Context, _ string) ([]Span, error) { return d.spans, nil }

func TestContacts_RejectsHandleNameAndCleansLinks(t *testing.T) {
	// A handle the model mis-tags as a person, a duplicate link, and a garbled model link
	// span that swallowed surrounding text — as seen on a real two-column CV.
	text := "@jprice_dev github.com/alex CONTACTS\n https://x.io"
	det := spansDetector{spans: []Span{
		{Start: 0, End: 10, Kind: KindName},         // "@jprice_dev" — not a real name
		{Start: 11, End: 26, Kind: KindLink},        // "github.com/alex" (dup of regex)
		{Start: 27, End: len(text), Kind: KindLink}, // "CONTACTS\n https://x.io" — garbled
	}}
	c := mustBuild(t, text, Contacts{}, det).Contacts()

	if c.FullName != "" {
		t.Errorf("FullName = %q, want empty (a @handle is not a name)", c.FullName)
	}
	seen := map[string]bool{}
	for _, l := range c.Links {
		if seen[l] {
			t.Errorf("duplicate link %q in %v", l, c.Links)
		}
		seen[l] = true
		if strings.ContainsAny(l, " \t\n") {
			t.Errorf("garbled link with whitespace: %q", l)
		}
	}
}

func TestContactsFromDetectedSpans(t *testing.T) {
	cv := "Ivan Petrov ivan@petrov.io github.com/ivanp linkedin.com/in/ivanp"
	r := mustBuild(t, cv, Contacts{}, nameDetector{names: []string{"Ivan Petrov"}})
	c := r.Contacts()
	if c.FullName != "Ivan Petrov" {
		t.Errorf("FullName = %q, want detected name", c.FullName)
	}
	if c.Email != "ivan@petrov.io" {
		t.Errorf("Email = %q, want detected email", c.Email)
	}
	if len(c.Links) != 2 {
		t.Errorf("Links = %v, want the two detected URLs", c.Links)
	}
}

func TestWordBoundaryAvoidsOverRedaction(t *testing.T) {
	text := "Mark shipped the benchmark on Mark's branch"
	r := mustBuild(t, text, Contacts{}, nameDetector{names: []string{"Mark"}})
	masked := r.Redact(text)
	if !strings.Contains(masked, "benchmark") {
		t.Fatalf("over-redacted 'benchmark': %s", masked)
	}
	if strings.Contains(masked, "Mark shipped") {
		t.Fatalf("standalone name not masked: %s", masked)
	}
}
