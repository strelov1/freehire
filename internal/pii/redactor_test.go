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
	cv := "Ilya Strelov\nstrelov1@gmail.com | github.com/strelov1\nSenior Engineer at RingCentral in London"
	r := mustBuild(t, cv, Contacts{}, nameDetector{names: []string{"Ilya Strelov"}})
	masked := r.Redact(cv)

	for _, leak := range []string{"Ilya Strelov", "strelov1@gmail.com", "github.com/strelov1"} {
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
	cv := "Ilya Strelov — strelov1@gmail.com — github.com/strelov1"
	r := mustBuild(t, cv, Contacts{}, nameDetector{names: []string{"Ilya Strelov"}})
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
	cv := "Ilya Strelov works remotely"
	structured := `{"full_name":"Ilya Strelov","email":"strelov1@gmail.com"}`
	known := Contacts{FullName: "Ilya Strelov", Email: "strelov1@gmail.com"}
	r := mustBuild(t, cv, known, nameDetector{names: []string{"Ilya Strelov"}})
	masked := r.Redact(structured)
	if strings.Contains(masked, "Ilya Strelov") || strings.Contains(masked, "strelov1@gmail.com") {
		t.Fatalf("known contacts leaked in structured blob: %s", masked)
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
