package cv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/ledongthuc/pdf"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
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
			{Role: "Senior Engineer", Company: "Analytical Engines", Start: &perioddate.PeriodDate{Year: 2018}, Current: true,
				Bullets: []string{"Cut latency by 40%."}},
		},
		Skills: []SkillGroup{{Group: "Languages", Items: []string{"Go", "Python", "SQL"}}},
	}
	tmpl, err := ResolveTemplate(DefaultTemplateID)
	if err != nil {
		t.Fatalf("resolve template: %v", err)
	}

	data, err := NewTypstRenderer(bin).Render(context.Background(), doc, tmpl, nil, LinkHrefs{})
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

	data, err := NewTypstRenderer(bin).Render(context.Background(), Document{}, tmpl, nil, LinkHrefs{})
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
			{Role: "Senior Engineer", Company: "Analytical Engines", Location: "London", Start: &perioddate.PeriodDate{Year: 2018}, Current: true,
				Summary: "Led core systems.", Bullets: []string{"Cut latency by 40%."}, Stack: []string{"Go", "Python"}},
		},
		Education: []EducationItem{{Degree: "BSc", Field: "CS", Institution: "Cambridge", Start: &perioddate.PeriodDate{Year: 2010}, End: &perioddate.PeriodDate{Year: 2014}}},
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
			data, err := r.Render(context.Background(), doc, tmpl, nil, LinkHrefs{})
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
			a, err := r.compile(context.Background(), tight, tmpl, "svg", nil, LinkHrefs{})
			if err != nil {
				t.Fatalf("compile tight: %v", err)
			}
			b, err := r.compile(context.Background(), wide, tmpl, "svg", nil, LinkHrefs{})
			if err != nil {
				t.Fatalf("compile wide: %v", err)
			}
			if bytes.Equal(a, b) {
				t.Errorf("template %q: margins had no effect on the rendered page", ti.ID)
			}
		})
	}
}

// TestRenderPayloadCarriesHasPhotoWithoutTouchingTheDocument proves the flag the templates
// branch on is produced at render time and inlined beside the document's own fields — so a
// client cannot set it through the CV endpoints and it never reaches storage.
func TestRenderPayloadCarriesHasPhoto(t *testing.T) {
	doc := Document{Header: Header{FullName: "Ada Lovelace"}}
	for _, hasPhoto := range []bool{true, false} {
		data, err := renderPayload(doc, hasPhoto, LinkHrefs{})
		if err != nil {
			t.Fatalf("renderPayload: %v", err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if got, ok := decoded["has_photo"].(bool); !ok || got != hasPhoto {
			t.Errorf("has_photo = %v (present: %v), want %v", decoded["has_photo"], ok, hasPhoto)
		}
		// The document's own fields must stay at the top level: the templates read
		// cv.header, not cv.document.header.
		if _, ok := decoded["header"]; !ok {
			t.Errorf("payload lost the document's fields: %s", data)
		}
	}
}

// TestRenderPayloadFormatsStructuredDatesToStrings is task 4.4/4.5's non-typst-dependent
// half: the templates' daterange-style helpers expect start/end/year as plain strings,
// exactly as they did before Start/End/Year became perioddate.PeriodDate — this proves that
// contract at the JSON boundary the templates actually read, without needing the typst
// binary installed.
func TestRenderPayloadFormatsStructuredDatesToStrings(t *testing.T) {
	doc := Document{
		Header: Header{FullName: "Ada Lovelace"},
		Experience: []ExperienceItem{
			{Role: "Senior Engineer", Company: "Analytical Engines",
				Start: &perioddate.PeriodDate{Year: 2018, Month: 3}, Current: true},
		},
		Education: []EducationItem{
			{Degree: "BSc", Institution: "Cambridge",
				Start: &perioddate.PeriodDate{Year: 2010}, End: &perioddate.PeriodDate{Year: 2014}},
		},
		Certifications: []Certification{
			{Name: "CKA", Issuer: "CNCF", Year: &perioddate.PeriodDate{Year: 2021}},
		},
	}
	data, err := renderPayload(doc, false, LinkHrefs{})
	if err != nil {
		t.Fatalf("renderPayload: %v", err)
	}
	var decoded struct {
		Experience []struct {
			Start string `json:"start"`
			End   string `json:"end"`
		} `json:"experience"`
		Education []struct {
			Start string `json:"start"`
			End   string `json:"end"`
		} `json:"education"`
		Certifications []struct {
			Year string `json:"year"`
		} `json:"certifications"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if got := decoded.Experience[0].Start; got != "Mar 2018" {
		t.Errorf("experience start = %q, want %q", got, "Mar 2018")
	}
	if got := decoded.Experience[0].End; got != "Present" {
		t.Errorf("experience end = %q, want %q (Current: true)", got, "Present")
	}
	if got := decoded.Education[0].Start; got != "2010" {
		t.Errorf("education start = %q, want %q", got, "2010")
	}
	if got := decoded.Education[0].End; got != "2014" {
		t.Errorf("education end = %q, want %q", got, "2014")
	}
	if got := decoded.Certifications[0].Year; got != "2021" {
		t.Errorf("certification year = %q, want %q", got, "2021")
	}
}

// TestPhotoIsStagedOnlyForPhotoTemplates is the whole point of the registry flag: the image
// must reach a template that prints it and must not be staged for one that does not.
func TestPhotoIsStagedOnlyForPhotoTemplates(t *testing.T) {
	bin, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst not installed; skipping photo staging test")
	}
	doc := Document{Header: Header{FullName: "Ada Lovelace"}, Summary: "Backend engineer."}
	r := NewTypstRenderer(bin)
	photo := testJPEG(t)

	for _, c := range []struct {
		template   string
		wantsImage bool
	}{
		{"portrait", true},
		{"headshot", true},
		{"classic-ats", false},
		{"sidebar", false},
	} {
		t.Run(c.template, func(t *testing.T) {
			tmpl, err := ResolveTemplate(c.template)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			with, err := r.compile(context.Background(), doc, tmpl, "svg", photo, LinkHrefs{})
			if err != nil {
				t.Fatalf("compile with photo: %v", err)
			}
			without, err := r.compile(context.Background(), doc, tmpl, "svg", nil, LinkHrefs{})
			if err != nil {
				t.Fatalf("compile without photo: %v", err)
			}
			// Typst inlines a raster into SVG as a base64 data URI, so its presence is a
			// direct assertion that the image was staged AND drawn.
			embedded := bytes.Contains(with, []byte("data:image/jpeg;base64"))
			if embedded != c.wantsImage {
				t.Errorf("template %q: image embedded = %v, want %v", c.template, embedded, c.wantsImage)
			}
			if c.wantsImage && bytes.Equal(with, without) {
				t.Errorf("template %q renders identically with and without a photo", c.template)
			}
			if !c.wantsImage && !bytes.Equal(with, without) {
				t.Errorf("template %q changed when handed a photo it should ignore", c.template)
			}
		})
	}
}

// A photo template with no stored headshot must still render — the placeholder path.
func TestPhotoTemplateRendersWithoutAPhoto(t *testing.T) {
	bin, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst not installed; skipping placeholder render test")
	}
	doc := Document{
		Header: Header{FullName: "Ada Lovelace", Email: "ada@example.com"},
		Skills: []SkillGroup{{Group: "Languages", Items: []string{"Go"}}},
	}
	for _, id := range []string{"portrait", "headshot"} {
		tmpl, err := ResolveTemplate(id)
		if err != nil {
			t.Fatalf("resolve %q: %v", id, err)
		}
		data, err := NewTypstRenderer(bin).Render(context.Background(), doc, tmpl, nil, LinkHrefs{})
		if err != nil {
			t.Fatalf("render %q without a photo: %v", id, err)
		}
		if !bytes.HasPrefix(data, []byte("%PDF")) {
			t.Fatalf("template %q: output is not a PDF", id)
		}
		if text := strings.ToLower(extractPDFText(t, data)); !strings.Contains(text, "ada lovelace") {
			t.Errorf("template %q: the placeholder render lost the text layer:\n%s", id, text)
		}
	}
}

// testJPEG is a small, real JPEG — the renderer stages bytes, so the test supplies bytes a
// decoder would accept rather than a stand-in string.
func testJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			img.Set(x, y, color.RGBA{R: uint8(4 * x), G: uint8(4 * y), B: 120, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

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

// A signature tripwire, not coverage. Render takes doc by value and Style holds only scalars,
// so today this cannot fail however the body is written — it fails the moment someone changes
// the parameter to *Document, which is exactly when the resolve-on-a-copy decision (the stored
// document must never hold an engine's face name) would silently stop holding.
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
	if _, err := NewTypstRenderer(bin).Render(context.Background(), doc, tmpl, nil, LinkHrefs{}); err != nil {
		t.Fatalf("render: %v", err)
	}
	if doc.Style.FontFamily != "tinos" {
		t.Errorf("render rewrote the caller's font family to %q; it must resolve on a copy", doc.Style.FontFamily)
	}
}

// TestEveryTemplateRendersLinksAsClickableLinks proves each registered template emits its
// links as PDF link annotations rather than as inert text. Registry-driven, so a template
// added later is held to the rule without this test being edited.
//
// This is a precondition of link tracing, not a cosmetic preference: tracing substitutes the
// link's target while leaving the visible text alone, so a template that prints a link as
// plain text carries no target to substitute and would report no clicks at all.
//
// classic-ats is the control. If every template fails, including it, the detector below is
// wrong rather than the templates.
func TestEveryTemplateRendersLinksAsClickableLinks(t *testing.T) {
	bin, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst not installed; skipping clickable-link render regression")
	}
	doc := Document{
		// Two header links, not one: a template that linked only the first would otherwise
		// pass, and this test exists to hold templates that do not exist yet.
		Header:     Header{FullName: "Ada Lovelace", Email: "ada@example.com", Links: []string{"github.com/ada", "linkedin.com/in/ada"}},
		Summary:    "Backend engineer with a decade of systems work.",
		Experience: []ExperienceItem{{Role: "Senior Engineer", Company: "Analytical Engines", Start: &perioddate.PeriodDate{Year: 2018}, Current: true, Bullets: []string{"Cut latency by 40%."}}},
		Projects:   []Project{{Name: "opensched", Link: "opensched.dev", Bullets: []string{"A tiny cron scheduler."}}},
		Skills:     []SkillGroup{{Group: "Languages", Items: []string{"Go"}}},
	}
	r := NewTypstRenderer(bin)
	for _, ti := range Templates() {
		t.Run(ti.ID, func(t *testing.T) {
			tmpl, err := ResolveTemplate(ti.ID)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			data, err := r.Render(context.Background(), doc, tmpl, nil, LinkHrefs{})
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			targets := pdfLinkTargets(t, data)
			for _, want := range []string{"github.com/ada", "linkedin.com/in/ada", "opensched.dev"} {
				if !linksTo(targets, want) {
					t.Errorf("template %q renders no link to %q — it prints the link as inert text, or "+
						"drops the section carrying it, and both leave nothing for tracing to substitute; "+
						"link targets found: %v", ti.ID, want, targets)
				}
			}
			// Clickable is not the same as followable. CVs store links scheme-less, and an
			// annotation carrying that verbatim is a relative URI that opens nothing.
			for _, got := range targets {
				if !strings.HasPrefix(got, "https://") && !strings.HasPrefix(got, "http://") {
					t.Errorf("template %q renders %q — a relative URI no reader can follow", ti.ID, got)
				}
			}
		})
	}
}

// linksTo reports whether any rendered link points at the given destination. Substring, not
// equality: what matters is which destination the link carries, not the exact spelling the
// payload hands to Typst.
func linksTo(targets []string, want string) bool {
	return slices.ContainsFunc(targets, func(got string) bool { return strings.Contains(got, want) })
}

// pdfLinkTargets returns the URI of every link annotation in the rendered PDF — what a
// reader actually follows, as opposed to what they see.
func pdfLinkTargets(t *testing.T, data []byte) []string {
	t.Helper()
	rd, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open rendered pdf: %v", err)
	}
	var targets []string
	for i := 1; i <= rd.NumPage(); i++ {
		annots := rd.Page(i).V.Key("Annots")
		for j := 0; j < annots.Len(); j++ {
			if uri := annots.Index(j).Key("A").Key("URI"); uri.Kind() == pdf.String {
				targets = append(targets, uri.RawString())
			}
		}
	}
	return targets
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

// TestRenderPayloadMakesEveryLinkAbsolute pins the fix for a defect that predates tracing: CVs
// store links the way a candidate writes them on paper ("github.com/ada"), and a PDF annotation
// carrying that verbatim is a *relative* URI no reader can follow. The payload is where it is
// normalised — not in six copies of Typst string handling.
func TestRenderPayloadMakesEveryLinkAbsolute(t *testing.T) {
	doc := Document{
		Header:   Header{Links: []string{"github.com/ada", "mailto:ada@example.com", "https://x.dev/a"}},
		Projects: []Project{{Name: "opensched", Link: "opensched.dev"}},
	}
	data, err := renderPayload(doc, false, LinkHrefs{})
	if err != nil {
		t.Fatalf("renderPayload: %v", err)
	}
	var payload struct {
		LinkHrefs LinkHrefs `json:"link_hrefs"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	want := []string{"https://github.com/ada", "", "https://x.dev/a"}
	if len(payload.LinkHrefs.Header) != len(want) {
		t.Fatalf("header hrefs = %q, want %q", payload.LinkHrefs.Header, want)
	}
	for i := range want {
		if payload.LinkHrefs.Header[i] != want[i] {
			t.Errorf("header href[%d] = %q, want %q", i, payload.LinkHrefs.Header[i], want[i])
		}
	}
	if got := payload.LinkHrefs.Projects; len(got) != 1 || got[0] != "https://opensched.dev" {
		t.Errorf("project hrefs = %q, want [https://opensched.dev]", got)
	}
}

// An href supplied by the caller — the traced URL minted for that position — replaces the
// normalised default. Anything the caller leaves empty keeps the default, so a link that cannot
// be traced still ends up absolute.
func TestSuppliedHrefsOverrideTheDefaults(t *testing.T) {
	doc := Document{Header: Header{Links: []string{"github.com/ada", "linkedin.com/in/ada"}}}
	data, err := renderPayload(doc, false, LinkHrefs{Header: []string{"https://freehire.me/cv/acme-x7abc"}})
	if err != nil {
		t.Fatalf("renderPayload: %v", err)
	}
	var payload struct {
		LinkHrefs LinkHrefs `json:"link_hrefs"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if got := payload.LinkHrefs.Header[0]; got != "https://freehire.me/cv/acme-x7abc" {
		t.Errorf("traced href[0] = %q, want the supplied one", got)
	}
	if got := payload.LinkHrefs.Header[1]; got != "https://linkedin.com/in/ada" {
		t.Errorf("href[1] = %q, want the normalised default", got)
	}
}

// The whole bargain of tracing: the reader sees the candidate's own link text and follows ours.
// If the substitution reached the visible text, the CV would advertise an opaque product URL
// where a recruiter expects github.com/name — and the ATS text layer would carry it too.
func TestATracedRenderSubstitutesTheTargetAndNotTheText(t *testing.T) {
	bin, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst not installed; skipping traced render test")
	}
	doc := Document{
		Header:     Header{FullName: "Ada Lovelace", Links: []string{"github.com/ada"}},
		Summary:    "Backend engineer.",
		Experience: []ExperienceItem{{Role: "Engineer", Company: "Acme", Start: &perioddate.PeriodDate{Year: 2018}, Current: true}},
	}
	const traced = "https://freehire.me/cv/acme-x7abc"
	tmpl, err := ResolveTemplate(DefaultTemplateID)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	data, err := NewTypstRenderer(bin).Render(context.Background(), doc, tmpl, nil,
		LinkHrefs{Header: []string{traced}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if !linksTo(pdfLinkTargets(t, data), traced) {
		t.Errorf("rendered link targets = %v, want the traced URL", pdfLinkTargets(t, data))
	}
	text := extractPDFText(t, data)
	if !strings.Contains(text, "github.com/ada") {
		t.Errorf("the reader no longer sees the candidate's own link:\n%s", text)
	}
	if strings.Contains(text, "freehire.me") {
		t.Error("the traced URL leaked into the visible text layer")
	}
}

// Every template must honour the substitution, or tracing would work in one design and silently
// report nothing in the others — the same hole the clickable-link guard exists to close.
func TestEveryTemplateHonoursASuppliedHref(t *testing.T) {
	bin, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst not installed; skipping per-template traced render test")
	}
	doc := Document{
		Header:   Header{FullName: "Ada Lovelace", Links: []string{"github.com/ada"}},
		Projects: []Project{{Name: "opensched", Link: "opensched.dev"}},
	}
	hrefs := LinkHrefs{
		Header:   []string{"https://freehire.me/cv/acme-aaaaa"},
		Projects: []string{"https://freehire.me/cv/acme-bbbbb"},
	}
	r := NewTypstRenderer(bin)
	for _, ti := range Templates() {
		t.Run(ti.ID, func(t *testing.T) {
			tmpl, err := ResolveTemplate(ti.ID)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			data, err := r.Render(context.Background(), doc, tmpl, nil, hrefs)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			targets := pdfLinkTargets(t, data)
			for _, want := range hrefs.Header {
				if !linksTo(targets, want) {
					t.Errorf("template %q ignored the traced header href; targets: %v", ti.ID, targets)
				}
			}
			for _, want := range hrefs.Projects {
				if !linksTo(targets, want) {
					t.Errorf("template %q ignored the traced project href; targets: %v", ti.ID, targets)
				}
			}
		})
	}
}
