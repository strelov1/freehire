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

	data, err := NewTypstRenderer(bin).Render(context.Background(), doc, tmpl, nil)
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

	data, err := NewTypstRenderer(bin).Render(context.Background(), Document{}, tmpl, nil)
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
			data, err := r.Render(context.Background(), doc, tmpl, nil)
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
			a, err := r.compile(context.Background(), tight, tmpl, "svg", nil)
			if err != nil {
				t.Fatalf("compile tight: %v", err)
			}
			b, err := r.compile(context.Background(), wide, tmpl, "svg", nil)
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
		data, err := renderPayload(doc, hasPhoto)
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
			with, err := r.compile(context.Background(), doc, tmpl, "svg", photo)
			if err != nil {
				t.Fatalf("compile with photo: %v", err)
			}
			without, err := r.compile(context.Background(), doc, tmpl, "svg", nil)
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
		data, err := NewTypstRenderer(bin).Render(context.Background(), doc, tmpl, nil)
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
