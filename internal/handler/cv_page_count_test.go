package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/cv"
)

// fakePDFWithPageCount builds the minimal bytes ledongthuc/pdf accepts as a PDF, with a page
// tree whose /Count is n — enough to drive NumPage() without a real page object per count, so
// a test can pin the counting logic without shelling out to typst.
func fakePDFWithPageCount(n int) []byte {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	off1 := len(header)
	off2 := off1 + len(obj1)
	obj2 := fmt.Sprintf("2 0 obj\n<< /Type /Pages /Kids [] /Count %d >>\nendobj\n", n)
	xrefOffset := off2 + len(obj2)
	xref := fmt.Sprintf("xref\n0 3\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \ntrailer\n<< /Size 3 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF",
		off1, off2, xrefOffset)
	return []byte(header + obj1 + obj2 + xref)
}

// fakePDFWithGarbageObject is a well-formed header/xref/trailer pointing object 1 at bytes
// that are not a valid PDF object. Resolving the Root indirect reference then panics deep in
// ledongthuc/pdf's object parser (a bare `panic()`, not a returned error) — this is the shape
// that would crash the unrecovered goroutine pdfPageCount runs in without its own recover.
func fakePDFWithGarbageObject() []byte {
	header := "%PDF-1.4\n"
	obj1 := "NOT AN OBJECT AT ALL ]]] <<\n"
	xrefOffset := len(header) + len(obj1)
	xref := fmt.Sprintf("xref\n0 2\n0000000000 65535 f \n%010d 00000 n \ntrailer\n<< /Size 2 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF",
		len(header), xrefOffset)
	return []byte(header + obj1 + xref)
}

func TestPDFPageCount_CountsARealPageTree(t *testing.T) {
	n, err := pdfPageCount(fakePDFWithPageCount(3))
	if err != nil {
		t.Fatalf("pdfPageCount: %v", err)
	}
	if n != 3 {
		t.Errorf("pages = %d, want 3", n)
	}
}

// TestPDFPageCount_RecoversFromLibraryPanic pins the defensive recover: without it, a Typst
// edge case this parser mishandles would panic out of pdfPageCount, and that panic runs
// unrecovered all the way up through the assistant tool loop's SSE stream-writer goroutine —
// crashing the whole server process, not just this request.
func TestPDFPageCount_RecoversFromLibraryPanic(t *testing.T) {
	_, err := pdfPageCount(fakePDFWithGarbageObject())
	if err == nil {
		t.Fatal("err = nil, want an error for a PDF whose Root object does not parse")
	}
}

// TestPDFPageCount_ZeroPagesIsAnError guards NumPage()'s own silent failure mode: it derives
// the count by walking Root -> Pages -> Count and returns a bare 0, with no error, for any
// page-tree shape it does not expect. Trusting that 0 would tell the tailoring agent an empty
// CV is a one-page CV.
func TestPDFPageCount_ZeroPagesIsAnError(t *testing.T) {
	_, err := pdfPageCount(fakePDFWithPageCount(0))
	if err == nil {
		t.Fatal("err = nil, want an error when the rendered pdf reports zero pages")
	}
}

func TestRenderedCVPageCount_NoRendererIsAnError(t *testing.T) {
	h := &cvHandlers{}
	tmpl, _ := cv.ResolveTemplate("classic-ats")

	if _, err := h.renderedCVPageCount(context.Background(), cv.Document{}, tmpl); !errors.Is(err, errNoRenderer) {
		t.Errorf("err = %v, want errNoRenderer", err)
	}
}

func TestRenderedCVPageCount_RenderFailureIsReported(t *testing.T) {
	boom := errors.New("typst exploded")
	h := &cvHandlers{cvRenderer: &fakeCVRenderer{err: boom}}
	tmpl, _ := cv.ResolveTemplate("classic-ats")

	if _, err := h.renderedCVPageCount(context.Background(), cv.Document{}, tmpl); !errors.Is(err, boom) {
		t.Errorf("err = %v, want it to wrap the renderer's error", err)
	}
}

func TestRenderedCVPageCount_ReadsTheRenderedPDF(t *testing.T) {
	h := &cvHandlers{cvRenderer: &fakeCVRenderer{pdf: fakePDFWithPageCount(2)}}
	tmpl, _ := cv.ResolveTemplate("classic-ats")

	n, err := h.renderedCVPageCount(context.Background(), cv.Document{}, tmpl)
	if err != nil {
		t.Fatalf("renderedCVPageCount: %v", err)
	}
	if n != 2 {
		t.Errorf("pages = %d, want 2", n)
	}
}

// TestCVPageCountTool_ReportsPages exercises the cv_page_count tool end to end over the same
// fakes cv_job_match_test.go uses, so it needs no typst binary and no database.
func TestCVPageCountTool_ReportsPages(t *testing.T) {
	repo := &cvRepo{id: testCVID, userID: 3, jobID: 9, data: []byte(oneExperienceCV)}
	h := &cvHandlers{
		cvStore:    cv.NewStore(repo),
		cvRenderer: &fakeCVRenderer{pdf: fakePDFWithPageCount(2)},
	}
	a := &assistantHandlers{cv: h}

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_page_count")
	out, err := tool.Run(context.Background(), 3, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("cv_page_count: %v", err)
	}
	payload, _ := json.Marshal(out)
	var got struct {
		Available bool `json:"available"`
		Pages     int  `json:"pages"`
	}
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal: %v (payload=%s)", err, payload)
	}
	if !got.Available {
		t.Fatalf("available = false, want true (payload=%s)", payload)
	}
	if got.Pages != 2 {
		t.Errorf("pages = %d, want 2", got.Pages)
	}
}

// TestCVPageCountTool_RenderFailureDegradesToUnavailable matches job_match's own convention:
// a scoring/rendering failure is reported as available:false with a reason, never a tool error
// that would end the agent's turn.
func TestCVPageCountTool_RenderFailureDegradesToUnavailable(t *testing.T) {
	repo := &cvRepo{id: testCVID, userID: 3, jobID: 9, data: []byte(oneExperienceCV)}
	h := &cvHandlers{
		cvStore:    cv.NewStore(repo),
		cvRenderer: &fakeCVRenderer{err: errors.New("typst exploded")},
	}
	a := &assistantHandlers{cv: h}

	tool := toolByName(t, a.assistantCVTools(testCVID, 9, uuid.New()), "cv_page_count")
	out, err := tool.Run(context.Background(), 3, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("cv_page_count: %v", err)
	}
	payload, _ := json.Marshal(out)
	var got struct {
		Available bool `json:"available"`
	}
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal: %v (payload=%s)", err, payload)
	}
	if got.Available {
		t.Errorf("available = true, want false when the render fails (payload=%s)", payload)
	}
}

// TestRenderedCVPageCount_RealToolchain exercises the real Typst binary, pinning that a
// two-page document renders to more than one page and a short one to exactly one. Skips
// without typst installed, same as the other real-toolchain tests in this package.
func TestRenderedCVPageCount_RealToolchain(t *testing.T) {
	bin, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst not installed; skipping real-toolchain page count")
	}
	h := &cvHandlers{cvRenderer: cv.NewTypstRenderer(bin)}
	tmpl, err := cv.ResolveTemplate("classic-ats")
	if err != nil {
		t.Fatalf("resolve classic-ats: %v", err)
	}

	short := cv.Document{
		Margins: cv.DefaultMargins(),
		Header:  cv.Header{FullName: "Ada Lovelace", Email: "ada@example.com", Phone: "+1 415 555 0134"},
		Summary: "Backend engineer.",
	}
	n, err := h.renderedCVPageCount(context.Background(), short, tmpl)
	if err != nil {
		t.Fatalf("renderedCVPageCount: %v", err)
	}
	if n != 1 {
		t.Errorf("pages = %d, want 1 for a short CV", n)
	}

	long := short
	// 16 of these dense entries is the observed tipping point into a 2nd page at
	// classic-ats's default 9.5pt/0.5in-margin layout; 20 keeps headroom so a small
	// template tweak doesn't make this borderline.
	long.Experience = make([]cv.ExperienceItem, 20)
	for i := range long.Experience {
		long.Experience[i] = cv.ExperienceItem{
			Role: "Senior Engineer", Company: "Analytical Engines", Start: "2010", End: "2020",
			Bullets: []string{
				"Built and operated distributed systems handling millions of requests per day.",
				"Led cross-functional initiatives spanning several engineering teams.",
				"Reduced infrastructure cost while improving reliability across the stack.",
			},
		}
	}
	n, err = h.renderedCVPageCount(context.Background(), long, tmpl)
	if err != nil {
		t.Fatalf("renderedCVPageCount: %v", err)
	}
	if n <= 1 {
		t.Errorf("pages = %d, want more than 1 for a dense multi-role CV", n)
	}
}
