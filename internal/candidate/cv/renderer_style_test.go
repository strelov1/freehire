package cv

import (
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
)

// absolutePointSize matches a `size:` given in points. The one legitimate occurrence is the
// preamble's own `ty("font_size", 9.5) * 1pt`, which is where the base comes from.
var absolutePointSize = regexp.MustCompile(`size:\s*[\d.]+\s*pt`)

// AGENTS.md tells the next template author that every internal size must be an em multiple of
// the base, and until now nothing enforced it: reintroducing `size: 12pt` broke no test, and
// the symptom — a heading that stops growing when the base size is raised — is the kind of
// thing nobody notices until a candidate complains their name looks wrong at 12pt.
//
// A source check rather than a render check, because it names the mistake precisely.
func TestTemplateSizesAreRelativeToTheBase(t *testing.T) {
	for _, ti := range Templates() {
		t.Run(ti.ID, func(t *testing.T) {
			tmpl, err := ResolveTemplate(ti.ID)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			for _, line := range strings.Split(string(tmpl.source), "\n") {
				if !absolutePointSize.MatchString(line) {
					continue
				}
				if strings.Contains(line, `ty("font_size"`) {
					continue // the preamble's own base, in points by definition
				}
				t.Errorf("absolute type size in %s.typ — use an em multiple of the base so it scales:\n  %s",
					ti.ID, strings.TrimSpace(line))
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
			{Role: "Senior Engineer", Company: "Analytical Engines", Start: &perioddate.PeriodDate{Year: 2018}, Current: true,
				Bullets: []string{"Cut latency by 40%."}, Stack: []string{"Go"}},
		},
		Skills: []SkillGroup{{Group: "Languages", Items: []string{"Go", "Python", "SQL"}}},
	}
}

// Every template must honour EACH of the three style values, not just the one it was developed
// against. The .typ files share no code by construction, so a preamble wired into five of them
// and forgotten in the sixth is the expected failure mode — and so is a preamble that reads the
// family but forgets the size. Each value is therefore asserted on its own: setting all three at
// once and checking the output merely differs would pass on any one of them working.
func TestStyledDocumentRendersInEveryTemplate(t *testing.T) {
	bin, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst not installed; skipping styled render test")
	}
	r := NewTypstRenderer(bin)
	// Long enough to wrap at every template's measure — modern-sans sets a wide one, and a
	// summary that fits on a single line there makes a leading change invisible.
	doc := styledDoc(Style{})
	doc.Summary = strings.Repeat("Backend engineer with a decade of systems work across payments, "+
		"settlement, and the infrastructure underneath them. ", 3)

	for _, ti := range Templates() {
		t.Run(ti.ID, func(t *testing.T) {
			tmpl, err := ResolveTemplate(ti.ID)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			render := func(s Style) []byte {
				d := doc
				d.Style = s
				out, err := r.compile(context.Background(), d, tmpl, "svg", nil, LinkHrefs{})
				if err != nil {
					t.Fatalf("render %+v: %v", s, err)
				}
				return out
			}
			// SVG, because a Typst PDF carries a creation timestamp and would differ anyway.
			base := render(Style{})
			for _, c := range []struct {
				what  string
				style Style
			}{
				{"font_size", Style{FontSize: 12.0}},
				{"line_height", Style{LineHeight: 0.9}},
			} {
				if bytes.Equal(base, render(c.style)) {
					t.Errorf("template %q ignores %s — its style preamble is missing or incomplete", ti.ID, c.what)
				}
			}

			// The face needs the PDF: Typst does not error on a font it cannot find, it
			// substitutes one, so the only way to tell "applied" from "silently ignored" is to
			// see the face embedded in the output.
			d := doc
			d.Style = Style{FontFamily: "carlito"}
			data, err := r.Render(context.Background(), d, tmpl, nil, LinkHrefs{})
			if err != nil {
				t.Fatalf("render pdf: %v", err)
			}
			if !bytes.HasPrefix(data, []byte("%PDF")) {
				t.Fatalf("output is not a PDF")
			}
			if !bytes.Contains(bytes.ToLower(data), []byte("carlito")) {
				t.Errorf("template %q ignores font_family — the chosen face is not embedded", ti.ID)
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

	small, err := r.Render(context.Background(), styledDoc(Style{FontSize: 8.5}), tmpl, nil, LinkHrefs{})
	if err != nil {
		t.Fatalf("render small: %v", err)
	}
	large, err := r.Render(context.Background(), styledDoc(Style{FontSize: 12.0}), tmpl, nil, LinkHrefs{})
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
	pSmall, err := r.Render(context.Background(), Document{Style: Style{FontSize: 8.5}}, probe, nil, LinkHrefs{})
	if err != nil {
		t.Fatalf("render probe small: %v", err)
	}
	pLarge, err := r.Render(context.Background(), Document{Style: Style{FontSize: 12.0}}, probe, nil, LinkHrefs{})
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
			styled, err := r.compile(context.Background(), doc, tmpl, "svg", nil, LinkHrefs{})
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
			bare, err := r.compile(context.Background(), doc, plain, "svg", nil, LinkHrefs{})
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
