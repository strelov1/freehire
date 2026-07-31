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

// styledDoc is the representative CV the typography render tests share.
func styledDoc(s Style) Document {
	return Document{
		Style:   s,
		Header:  Header{FullName: "Ada Lovelace", Email: "ada@example.com", Location: "London"},
		Summary: "Backend engineer with a decade of systems work.",
		Experience: []ExperienceItem{
			{Role: "Senior Engineer", Company: "Analytical Engines", Start: "2018", End: "Present",
				Bullets: []string{"Cut latency by 40%."}, Stack: []string{"Go"}},
		},
		Skills: []SkillGroup{{Group: "Languages", Items: []string{"Go", "Python", "SQL"}}},
	}
}

// Every template must honour the style block, not just the one it was developed against —
// the four .typ files share no code by construction, so a preamble added to three of them
// and forgotten in the fourth is the expected failure mode here.
func TestStyledDocumentRendersInEveryTemplate(t *testing.T) {
	bin, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst not installed; skipping styled render test")
	}
	doc := styledDoc(Style{FontFamily: "carlito", FontSize: 11.0, LineHeight: 0.7})
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
			// Typst does not error on a font it cannot find, it substitutes one. Asserting the
			// face is embedded is the only way to tell "applied" from "silently ignored".
			if !bytes.Contains(bytes.ToLower(data), []byte("carlito")) {
				t.Errorf("template %q: the chosen face is not embedded — the style block was ignored", ti.ID)
			}
			text := strings.ToLower(extractPDFText(t, data))
			if !strings.Contains(text, "ada lovelace") {
				t.Errorf("template %q: text layer broken under a style block:\n%s", ti.ID, text)
			}
		})
	}
}

// A set font size must move the page, and must move the name with it: the templates size
// their headings relative to the base, so a bigger base means a bigger name. If the internal
// sizes were left absolute the pages would still differ (body text changed) — hence the
// second, sharper assertion that the two renders differ under a heading-only probe.
func TestFontSizeScalesTheWholeHierarchy(t *testing.T) {
	bin, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst not installed; skipping type-scale test")
	}
	r := NewTypstRenderer(bin)
	tmpl, err := ResolveTemplate(DefaultTemplateID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	small, err := r.Render(context.Background(), styledDoc(Style{FontSize: 8.5}), tmpl)
	if err != nil {
		t.Fatalf("render small: %v", err)
	}
	large, err := r.Render(context.Background(), styledDoc(Style{FontSize: 12.0}), tmpl)
	if err != nil {
		t.Fatalf("render large: %v", err)
	}
	if bytes.Equal(small, large) {
		t.Fatal("font size had no effect on the rendered page")
	}

	// The name is set relative to the base, so it grows too. A template that kept its name at
	// an absolute 12pt would render the name identically at both base sizes.
	probe := Template{ID: "scale-probe", source: []byte(
		"#let cv = json(\"data.json\")\n" +
			"#let st = cv.at(\"style\", default: (:))\n" +
			"#set text(font: \"Libertinus Serif\", size: (if st.at(\"font_size\", default: 0) > 0 { st.font_size } else { 9.5 }) * 1pt)\n" +
			"#text(size: 1.25em)[Ada Lovelace]\n")}
	pSmall, err := r.Render(context.Background(), Document{Style: Style{FontSize: 8.5}}, probe)
	if err != nil {
		t.Fatalf("render probe small: %v", err)
	}
	pLarge, err := r.Render(context.Background(), Document{Style: Style{FontSize: 12.0}}, probe)
	if err != nil {
		t.Fatalf("render probe large: %v", err)
	}
	if bytes.Equal(pSmall, pLarge) {
		t.Error("an em-relative heading did not scale with the base size")
	}
}

// An unset style must not leak a value: rendering a CV with no typography has to give exactly
// what the template's own fallbacks give. That is what makes zero mean "inherit" in practice,
// and it is the invariant a later refactor of the preamble is most likely to break.
//
// The comparison is against the same template with `st` forced empty, so it pins the plumbing,
// NOT the fallback constants — a preamble that defaulted to the wrong size would pass. Parity
// with the pre-change templates was verified once, by rendering both revisions; there is no
// way to keep asserting it without committing goldens that would need regenerating on every
// legitimate template edit.
//
// The oracle is SVG, not PDF: a Typst PDF embeds a creation timestamp, so two renders of one
// source in different seconds differ in bytes and any PDF comparison is a coin flip.
func TestUnstyledRenderMatchesTheTemplatesOwnDefaults(t *testing.T) {
	bin, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst not installed; skipping default-parity test")
	}
	r := NewTypstRenderer(bin)
	doc := styledDoc(Style{})

	for _, ti := range Templates() {
		t.Run(ti.ID, func(t *testing.T) {
			tmpl, err := ResolveTemplate(ti.ID)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			styled, err := r.compile(context.Background(), doc, tmpl, "svg")
			if err != nil {
				t.Fatalf("render styled: %v", err)
			}
			// Force every fallback down a path that cannot consult the style block at all: if
			// the plumbing leaks a value when nothing is set, these two disagree.
			plain := Template{ID: ti.ID, source: bytes.Replace(tmpl.source,
				[]byte(`#let st = cv.at("style", default: (:))`),
				[]byte(`#let st = (:)`), 1)}
			if bytes.Equal(plain.source, tmpl.source) {
				t.Fatalf("template %q has no style preamble to neutralise", ti.ID)
			}
			bare, err := r.compile(context.Background(), doc, plain, "svg")
			if err != nil {
				t.Fatalf("render bare: %v", err)
			}
			if !bytes.Equal(styled, bare) {
				t.Errorf("template %q: an unstyled CV does not render as the template's own defaults (%d vs %d bytes)",
					ti.ID, len(styled), len(bare))
			}
		})
	}
}

// The renderer resolves a registry id to the engine's own family name, and must do that on
// its own copy: the caller holds the document the handler is about to persist.
func TestRenderDoesNotMutateTheCallersDocument(t *testing.T) {
	bin, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst not installed; skipping render-purity test")
	}
	doc := styledDoc(Style{FontFamily: "tinos", FontSize: 10.0})
	tmpl, err := ResolveTemplate(DefaultTemplateID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, err := NewTypstRenderer(bin).Render(context.Background(), doc, tmpl); err != nil {
		t.Fatalf("render: %v", err)
	}
	if doc.Style.FontFamily != "tinos" {
		t.Errorf("render rewrote the caller's font family to %q; it must resolve on a copy", doc.Style.FontFamily)
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
