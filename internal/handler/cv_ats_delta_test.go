package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/atscheck"
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/resume"
)

// fakeCVRenderer stands in for the Typst renderer: it records what it was asked to
// render and returns canned bytes, so a scorer test needs no typst binary.
type fakeCVRenderer struct {
	pdf []byte
	err error
	// renderFn, when set, derives the bytes from the document, so a test can tell the two
	// sides of a comparison apart by content rather than by call order.
	renderFn  func(cv.Document) []byte
	gotTmpl   cv.Template
	calls     int
	docsSeen  []cv.Document
	tmplsSeen []cv.Template
	// photosSeen records the headshot handed to each call, so a test can assert that the
	// text-only scoring path asks for none.
	photosSeen [][]byte
}

func (f *fakeCVRenderer) Render(_ context.Context, doc cv.Document, tmpl cv.Template, photo []byte, hrefs cv.LinkHrefs) ([]byte, error) {
	_ = hrefs
	f.calls++
	f.gotTmpl = tmpl
	f.docsSeen = append(f.docsSeen, doc)
	f.tmplsSeen = append(f.tmplsSeen, tmpl)
	f.photosSeen = append(f.photosSeen, photo)
	if f.err != nil {
		return nil, f.err
	}
	if f.renderFn != nil {
		return f.renderFn(doc), nil
	}
	return f.pdf, nil
}

// textFromPDF fakes the poppler text-layer extraction by treating the "PDF" bytes as
// their own text, so a test states the text layer directly.
func textFromPDF(b []byte) (string, error) { return string(b), nil }

const scorableCV = "Jane Roe\njane@example.com  +1 415 555 0134\n\nSummary\nBackend engineer. Core stack: Golang, Kafka, Kubernetes.\n\nExperience\n2020-2024 Acme — Senior Engineer\n- Built Go services handling 2M requests/day\n- Led a team of 4 on Kubernetes migration\n\nEducation\n2016-2020 BSc Computer Science\n\nSkills\nGolang, Kafka, Kubernetes, PostgreSQL"

func TestScoreRenderedCV_ScoresTheRenderedTextLayer(t *testing.T) {
	r := &fakeCVRenderer{pdf: []byte(scorableCV)}
	h := &cvHandlers{cvRenderer: r, extractPDFText: textFromPDF}
	tmpl, err := cv.ResolveTemplate("classic-ats")
	if err != nil {
		t.Fatalf("ResolveTemplate: %v", err)
	}

	report, err := h.scoreRenderedCV(context.Background(), cv.Document{Summary: "ignored by the fake"}, tmpl, []string{"go", "kafka", "terraform"})
	if err != nil {
		t.Fatalf("scoreRenderedCV: %v", err)
	}

	if report.Overall <= 0 {
		t.Errorf("overall = %d, want a positive score for a clean CV text", report.Overall)
	}
	if r.calls != 1 {
		t.Errorf("render calls = %d, want 1", r.calls)
	}
	if r.gotTmpl.ID != "classic-ats" {
		t.Errorf("rendered with template %q, want classic-ats", r.gotTmpl.ID)
	}
	// The score is read off the text layer, where a portrait contributes nothing — so
	// this path must not pay for a headshot fetch, whatever template is being scored.
	for i, photo := range r.photosSeen {
		if photo != nil {
			t.Errorf("render call %d was handed a %d-byte headshot; ATS scoring is text-only", i, len(photo))
		}
	}
}

func TestScoreRenderedCV_KeywordBaselineNamesTheMissingSkill(t *testing.T) {
	h := &cvHandlers{cvRenderer: &fakeCVRenderer{pdf: []byte(scorableCV)}, extractPDFText: textFromPDF}
	tmpl, _ := cv.ResolveTemplate("classic-ats")

	report, err := h.scoreRenderedCV(context.Background(), cv.Document{}, tmpl, []string{"go", "kafka", "terraform"})
	if err != nil {
		t.Fatalf("scoreRenderedCV: %v", err)
	}

	if !strings.Contains(strings.Join(report.RecommendedKeywords, ","), "terraform") {
		t.Errorf("recommended = %v, want terraform — it is in the baseline and absent from the CV", report.RecommendedKeywords)
	}
	if len(report.StrongKeywords) == 0 {
		t.Errorf("strong = %v, want the baseline skills the CV text does carry", report.StrongKeywords)
	}
}

func TestScoreRenderedCV_SkillsComeFromTheRenderedTextNotTheDocument(t *testing.T) {
	// The document claims Rust; the rendered text layer does not mention it. The score
	// describes the artifact, so Rust must not count as covered.
	doc := cv.Document{Skills: []cv.SkillGroup{{Group: "Languages", Items: []string{"Rust"}}}}
	h := &cvHandlers{cvRenderer: &fakeCVRenderer{pdf: []byte(scorableCV)}, extractPDFText: textFromPDF}
	tmpl, _ := cv.ResolveTemplate("classic-ats")

	report, err := h.scoreRenderedCV(context.Background(), doc, tmpl, []string{"rust"})
	if err != nil {
		t.Fatalf("scoreRenderedCV: %v", err)
	}

	if strings.Contains(strings.ToLower(strings.Join(report.StrongKeywords, ",")), "rust") {
		t.Errorf("strong = %v, want rust absent — it is in the document but not in the rendered text", report.StrongKeywords)
	}
}

func TestScoreRenderedCV_RenderFailureIsReported(t *testing.T) {
	boom := errors.New("typst exploded")
	h := &cvHandlers{cvRenderer: &fakeCVRenderer{err: boom}, extractPDFText: textFromPDF}
	tmpl, _ := cv.ResolveTemplate("classic-ats")

	_, err := h.scoreRenderedCV(context.Background(), cv.Document{}, tmpl, nil)

	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want it to wrap the renderer's error", err)
	}
}

func TestScoreRenderedCV_ExtractionFailureIsReported(t *testing.T) {
	boom := errors.New("pdftotext missing")
	h := &cvHandlers{
		cvRenderer:     &fakeCVRenderer{pdf: []byte("whatever")},
		extractPDFText: func([]byte) (string, error) { return "", boom },
	}
	tmpl, _ := cv.ResolveTemplate("classic-ats")

	_, err := h.scoreRenderedCV(context.Background(), cv.Document{}, tmpl, nil)

	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want it to wrap the extractor's error", err)
	}
}

// TestScoreRenderedCV_AFieldTheTemplateDropsDoesNotScore is the claim the whole design
// rests on, checked against the real toolchain: the score describes the rendered artifact,
// so a document field the active template never renders cannot earn points. `sidebar` has
// no certifications block while `classic-ats` does, so the same document — whose only
// mention of Kubernetes is a certification — must score that keyword under one and miss it
// under the other. Skips when typst or pdftotext is absent.
func TestScoreRenderedCV_AFieldTheTemplateDropsDoesNotScore(t *testing.T) {
	bin, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst not installed; skipping rendered-artifact scoring")
	}
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not installed; skipping rendered-artifact scoring")
	}
	h := &cvHandlers{cvRenderer: cv.NewTypstRenderer(bin), extractPDFText: resume.ExtractPDFText}

	// Kubernetes appears in the certification and nowhere else in the document.
	doc := cv.Document{
		Margins: cv.DefaultMargins(),
		Header:  cv.Header{FullName: "Ada Lovelace", Email: "ada@example.com", Phone: "+1 415 555 0134"},
		Summary: "Backend engineer with a decade of systems work.",
		Experience: []cv.ExperienceItem{
			{Role: "Senior Engineer", Company: "Analytical Engines", Start: "2018", End: "Present",
				Bullets: []string{"Cut p99 latency by 40% across 12 services."}},
		},
		Education: []cv.EducationItem{{Degree: "BSc Mathematics", Institution: "Cambridge", Start: "2010", End: "2014"}},
		Skills:    []cv.SkillGroup{{Group: "Languages", Items: []string{"Go", "Python", "SQL"}}},
		Certifications: []cv.Certification{
			{Name: "Certified Kubernetes Administrator", Issuer: "CNCF", Year: "2023"},
		},
	}
	classic, err := cv.ResolveTemplate("classic-ats")
	if err != nil {
		t.Fatalf("resolve classic-ats: %v", err)
	}
	sidebar, err := cv.ResolveTemplate("sidebar")
	if err != nil {
		t.Fatalf("resolve sidebar: %v", err)
	}

	withCerts, err := h.scoreRenderedCV(context.Background(), doc, classic, []string{"kubernetes"})
	if err != nil {
		t.Fatalf("score under classic-ats: %v", err)
	}
	withoutCerts, err := h.scoreRenderedCV(context.Background(), doc, sidebar, []string{"kubernetes"})
	if err != nil {
		t.Fatalf("score under sidebar: %v", err)
	}

	if !containsFold(withCerts.StrongKeywords, "kubernetes") {
		t.Errorf("classic-ats strong = %v, want kubernetes — the template renders the certification that carries it",
			withCerts.StrongKeywords)
	}
	if containsFold(withoutCerts.StrongKeywords, "kubernetes") {
		t.Errorf("sidebar strong = %v, want kubernetes absent — the template drops the certification that carries it",
			withoutCerts.StrongKeywords)
	}
}

// TestCVRegister_ATSDeltaIsCookieOnly pins the gate on the delta read against the real
// register(). Cookie-only is the enforcement, not a preference: the tailoring agent
// authenticates with a CLI credential, so a cookie-only route is what keeps the score away
// from the thing being measured. Widening this to `key` or `cvKey` hands the agent a metric
// to optimise, and this test is the tripwire.
func TestCVRegister_ATSDeltaIsCookieOnly(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api/v1")
	(&cvHandlers{}).register(api, middleware{
		key:    namedGate("key"),
		cvKey:  namedGate("cvKey"),
		cookie: namedGate("cookie"),
	})

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/me/cvs/"+uuid.New().String()+"/ats-delta", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if got := string(body); got != "cookie" {
		t.Errorf("GET /me/cvs/:id/ats-delta is gated by %q, want %q", got, "cookie")
	}
}

// TestScoreRenderedCV_RealToolchainDelta is the only test that exercises the whole scoring
// path with the real binaries: two documents through typst and pdftotext into Compare. It
// checks both directions the pipeline can be broken in — identical documents must yield a
// zero delta (a nondeterministic render or a truncated extraction would not), and a document
// that gains the vacancy's evidence must score above the one that lacks it (an extraction
// that silently returned nothing would score both the same). Skips without the binaries.
func TestScoreRenderedCV_RealToolchainDelta(t *testing.T) {
	bin, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst not installed; skipping real-toolchain delta")
	}
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not installed; skipping real-toolchain delta")
	}
	h := &cvHandlers{cvRenderer: cv.NewTypstRenderer(bin), extractPDFText: resume.ExtractPDFText}
	tmpl, err := cv.ResolveTemplate("classic-ats")
	if err != nil {
		t.Fatalf("resolve template: %v", err)
	}
	keywords := []string{"go", "kafka"}

	base := cv.Document{
		Margins: cv.DefaultMargins(),
		Header:  cv.Header{FullName: "Ada Lovelace", Email: "ada@example.com", Phone: "+1 415 555 0134"},
		Summary: "Backend engineer.",
		Experience: []cv.ExperienceItem{
			{Role: "Senior Engineer", Company: "Analytical Engines", Start: "2018", End: "Present",
				Bullets: []string{"Built Go services."}},
		},
		Education: []cv.EducationItem{{Degree: "BSc Mathematics", Institution: "Cambridge", Start: "2010", End: "2014"}},
		Skills:    []cv.SkillGroup{{Group: "Languages", Items: []string{"Go"}}},
	}
	tailored := base
	tailored.Summary = "Backend engineer. Core stack: Go, Kafka."
	tailored.Experience = []cv.ExperienceItem{
		{Role: "Senior Engineer", Company: "Analytical Engines", Start: "2018", End: "Present",
			Bullets: []string{"Built Go services handling 2M requests/day.", "Ran Kafka pipelines for 4 teams."}},
	}
	tailored.Skills = []cv.SkillGroup{{Group: "Languages", Items: []string{"Go", "Kafka"}}}

	baseReport, err := h.scoreRenderedCV(context.Background(), base, tmpl, keywords)
	if err != nil {
		t.Fatalf("score base: %v", err)
	}
	tailoredReport, err := h.scoreRenderedCV(context.Background(), tailored, tmpl, keywords)
	if err != nil {
		t.Fatalf("score tailored: %v", err)
	}

	if d := atscheck.Compare(baseReport, baseReport); d.Change != 0 || len(d.Categories) == 0 {
		t.Errorf("self-comparison = change %d over %d categories, want 0 over the scorer's categories",
			d.Change, len(d.Categories))
	}
	d := atscheck.Compare(baseReport, tailoredReport)
	if d.Change <= 0 {
		t.Errorf("change = %d (base %d → tailored %d), want positive: the tailored document adds the vacancy's evidence",
			d.Change, d.Base, d.Tailored)
	}
	if d.Regressed {
		t.Errorf("regressed = true with change %d, want false", d.Change)
	}
	for _, c := range d.Categories {
		if c.Change != c.Tailored-c.Base {
			t.Errorf("category %s: change %d != tailored %d − base %d", c.ID, c.Change, c.Tailored, c.Base)
		}
	}
}

func containsFold(values []string, want string) bool {
	for _, v := range values {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

func TestScoreRenderedCV_NoRendererIsAnError(t *testing.T) {
	h := &cvHandlers{extractPDFText: textFromPDF}
	tmpl, _ := cv.ResolveTemplate("classic-ats")

	if _, err := h.scoreRenderedCV(context.Background(), cv.Document{}, tmpl, nil); err == nil {
		t.Error("err = nil, want an error when no renderer is configured")
	}
}

// TestScoreRenderedCV_NoExtractorIsAnError guards the asymmetry that would otherwise be a
// panic: a handler assembled with a renderer but no text extractor exists today
// (cv_integration_test.go sets cvRenderer on a struct literal), so calling the extractor
// unchecked turns a misassembled handler into a 500 instead of an unavailable delta.
func TestScoreRenderedCV_NoExtractorIsAnError(t *testing.T) {
	h := &cvHandlers{cvRenderer: &fakeCVRenderer{pdf: []byte(scorableCV)}}
	tmpl, _ := cv.ResolveTemplate("classic-ats")

	if _, err := h.scoreRenderedCV(context.Background(), cv.Document{}, tmpl, nil); err == nil {
		t.Error("err = nil, want an error when no text extractor is configured")
	}
}
