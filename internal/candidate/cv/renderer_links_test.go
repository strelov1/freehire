package cv

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/ledongthuc/pdf"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
)

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
