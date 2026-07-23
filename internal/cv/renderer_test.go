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

// TestRendererResolvesBundledSansFont proves the renderer makes the bundled Liberation Sans
// available under --ignore-system-fonts. Typst silently falls back (no error) when a font is
// missing, so success alone proves nothing — instead we assert the chosen face is actually
// embedded in the PDF, which only happens when --font-path points at the staged fonts.
func TestRendererResolvesBundledSansFont(t *testing.T) {
	bin, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst not installed; skipping bundled-font render test")
	}
	tmpl := Template{ID: "sans-probe", source: []byte(
		"#set text(font: \"Liberation Sans\")\nAda Lovelace — backend engineer\n")}

	data, err := NewTypstRenderer(bin).Render(context.Background(), Document{}, tmpl)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !bytes.Contains(bytes.ToLower(data), []byte("liberation")) {
		t.Error("rendered PDF does not embed Liberation Sans — --font-path wiring is missing (typst fell back to a default face)")
	}
}

// TestAllTemplatesProduceExtractableText renders every registered template against the same
// representative CV and asserts the text layer carries the name and a skill — including the
// non-ATS-safe sidebar, whose text must stay extractable even if column order is not linear.
func TestAllTemplatesProduceExtractableText(t *testing.T) {
	bin, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst not installed; skipping all-templates render regression")
	}
	doc := Document{
		Header:  Header{FullName: "Ada Lovelace", Email: "ada@example.com", Phone: "+1 555", Location: "London", Links: []string{"github.com/ada"}},
		Summary: "Backend engineer with a decade of systems work.",
		Experience: []ExperienceItem{
			{Role: "Senior Engineer", Company: "Analytical Engines", Location: "London", Start: "2018", End: "Present",
				Summary: "Led core systems.", Bullets: []string{"Cut latency by 40%."}, Stack: []string{"Go", "Python"}},
		},
		Education: []EducationItem{{Degree: "BSc", Field: "CS", Institution: "Cambridge", Start: "2010", End: "2014"}},
		Skills:    []SkillGroup{{Group: "Languages", Items: []string{"Go", "Python", "SQL"}}},
		Languages: []Language{{Name: "English", Level: "Native"}},
	}
	r := NewTypstRenderer(bin)
	for _, ti := range Templates() {
		t.Run(ti.ID, func(t *testing.T) {
			tmpl, err := ResolveTemplate(ti.ID)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			data, err := r.Render(context.Background(), doc, tmpl)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if !bytes.HasPrefix(data, []byte("%PDF")) {
				t.Fatalf("output is not a PDF")
			}
			// Case-insensitive: some templates uppercase the name for style, which is fine —
			// ATS parsers normalize case; what matters is the text is present and selectable.
			text := strings.ToLower(extractPDFText(t, data))
			for _, want := range []string{"ada lovelace", "python"} {
				if !strings.Contains(text, want) {
					t.Errorf("template %q: extracted text missing %q:\n%s", ti.ID, want, text)
				}
			}
		})
	}
}

// TestRenderAppliesMargins proves each template reads the document's page margins rather
// than hardcoding them: rendering the same CV with tight vs. wide margins must change the
// output. Compares SVG (deterministic, unlike PDF which embeds a creation timestamp) via
// the internal compile so a byte-equality check is meaningful.
func TestRenderAppliesMargins(t *testing.T) {
	bin, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst not installed; skipping margin render test")
	}
	doc := Document{
		Header:  Header{FullName: "Ada Lovelace", Email: "ada@example.com", Location: "London"},
		Summary: "Backend engineer with a decade of systems work.",
	}
	tight := doc
	tight.Margins = Margins{Top: 0.25, Right: 0.25, Bottom: 0.25, Left: 0.25}
	wide := doc
	wide.Margins = Margins{Top: 1.5, Right: 1.5, Bottom: 1.5, Left: 1.5}

	r := NewTypstRenderer(bin)
	for _, ti := range Templates() {
		t.Run(ti.ID, func(t *testing.T) {
			tmpl, err := ResolveTemplate(ti.ID)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			a, err := r.compile(context.Background(), tight, tmpl, "svg")
			if err != nil {
				t.Fatalf("compile tight: %v", err)
			}
			b, err := r.compile(context.Background(), wide, tmpl, "svg")
			if err != nil {
				t.Fatalf("compile wide: %v", err)
			}
			if bytes.Equal(a, b) {
				t.Errorf("template %q: margins had no effect on the rendered page", ti.ID)
			}
		})
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
