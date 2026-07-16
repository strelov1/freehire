package cv

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/ledongthuc/pdf"
)

func TestResolveTemplateDefaultsAndRejectsUnknown(t *testing.T) {
	tmpl, err := ResolveTemplate("")
	if err != nil {
		t.Fatalf("empty id should default, got: %v", err)
	}
	if tmpl.ID != DefaultTemplateID || len(tmpl.source) == 0 {
		t.Errorf("default template not resolved: id=%q sourceLen=%d", tmpl.ID, len(tmpl.source))
	}

	if _, err := ResolveTemplate(DefaultTemplateID); err != nil {
		t.Errorf("known id rejected: %v", err)
	}

	if _, err := ResolveTemplate("does-not-exist"); !errors.Is(err, ErrUnknownTemplate) {
		t.Errorf("unknown id err = %v, want ErrUnknownTemplate", err)
	}
}

func TestNewTypstRendererDisabledWithoutBinary(t *testing.T) {
	if r := NewTypstRenderer(""); r != nil {
		t.Errorf("empty bin should yield a nil (disabled) renderer, got %v", r)
	}
}

// TestTypstRendererProducesExtractableATSText is the ATS regression: a rendered CV must
// carry a selectable text layer containing the candidate's name and skills. It runs only
// when the typst binary is available (locally and in the prod image); elsewhere it skips.
func TestTypstRendererProducesExtractableATSText(t *testing.T) {
	bin, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst not installed; skipping ATS render regression")
	}

	doc := Document{
		Header:  Header{FullName: "Ada Lovelace", Email: "ada@example.com"},
		Summary: "Backend engineer with a decade of systems work.",
		Experience: []ExperienceItem{
			{Role: "Senior Engineer", Company: "Analytical Engines", Start: "2018", End: "Present",
				Bullets: []string{"Cut latency by 40%."}},
		},
		Skills: []SkillGroup{{Group: "Languages", Items: []string{"Go", "Python", "SQL"}}},
	}
	tmpl, err := ResolveTemplate(DefaultTemplateID)
	if err != nil {
		t.Fatalf("resolve template: %v", err)
	}

	data, err := NewTypstRenderer(bin).Render(context.Background(), doc, tmpl)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Fatalf("output is not a PDF (prefix %q)", data[:min(4, len(data))])
	}

	text := extractPDFText(t, data)
	for _, want := range []string{"Ada Lovelace", "Backend engineer with a decade", "Python", "Cut latency by 40%."} {
		if !strings.Contains(text, want) {
			t.Errorf("extracted text is missing %q (ATS layer broken):\n%s", want, text)
		}
	}
}

func extractPDFText(t *testing.T, data []byte) string {
	t.Helper()
	rd, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open rendered pdf: %v", err)
	}
	tr, err := rd.GetPlainText()
	if err != nil {
		t.Fatalf("extract text: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, tr); err != nil {
		t.Fatalf("read text: %v", err)
	}
	return buf.String()
}
