package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/identity/auth"

	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
)

// fakeBank records what the upload path handed the experience bank. The mapping itself is
// tested in internal/candidate/experience; what matters here is whether the import ran at all, and
// that it never surfaces to the upload when it cannot.
type fakeBank struct {
	calls     int
	entries   []experience.ImportEntry
	sourceRef string
	err       error
	history   []resumeextract.Experience
	projects  []resumeextract.Project
}

func (f *fakeBank) SeedHistory(context.Context, int64) (experience.SeedHistory, error) {
	return experience.SeedHistory{
		Experience:            f.history,
		Projects:              f.projects,
		HasJobEmployments:     len(f.history) > 0,
		HasProjectEmployments: len(f.projects) > 0,
	}, nil
}

func (f *fakeBank) Import(_ context.Context, _ int64, entries []experience.ImportEntry, sourceRef string) (experience.ImportResult, error) {
	f.calls++
	f.entries = entries
	f.sourceRef = sourceRef
	return experience.ImportResult{}, f.err
}

func bankFixture() resumeextract.Structured {
	return resumeextract.Structured{
		Experience: []resumeextract.Experience{
			{Title: "Senior Software Engineer", Company: "RingCentral", Current: true,
				Highlights: []string{"Cut latency 20s to 1s"}},
		},
	}
}

func TestImportExperienceRunsOnASuccessfulExtract(t *testing.T) {
	bank := &fakeBank{}
	h := &resumeHandlers{bank: bank}

	h.importExperience(context.Background(), 7, bankFixture(), "2026-07-28T10:00:00Z")

	if bank.calls != 1 {
		t.Fatalf("bank.calls = %d, want 1", bank.calls)
	}
	if bank.sourceRef != "2026-07-28T10:00:00Z" {
		t.Errorf("sourceRef = %q, want the upload stamp", bank.sourceRef)
	}
	if len(bank.entries) != 1 {
		t.Errorf("entries = %d, want 1", len(bank.entries))
	}
}

// The bank is best-effort exactly like the rest of the derive path: an unconfigured bank
// or a failing import must never surface to the upload.
func TestImportExperienceIsBestEffort(t *testing.T) {
	h := &resumeHandlers{bank: nil}
	h.importExperience(context.Background(), 7, bankFixture(), "stamp") // must not panic

	failing := &fakeBank{err: errors.New("database down")}
	h = &resumeHandlers{bank: failing}
	h.importExperience(context.Background(), 7, bankFixture(), "stamp")
	if failing.calls != 1 {
		t.Errorf("bank.calls = %d, want the failure swallowed after one attempt", failing.calls)
	}
}

// An extraction that produced no work history has nothing to bank; the import must not
// run at all rather than call with an empty slice.
func TestImportExperienceSkipsAnEmptyStructure(t *testing.T) {
	bank := &fakeBank{}
	h := &resumeHandlers{bank: bank}

	h.importExperience(context.Background(), 7, resumeextract.Structured{FullName: "Ilya Strelov"}, "stamp")

	if bank.calls != 0 {
		t.Errorf("bank.calls = %d, want 0 — nothing was extracted to bank", bank.calls)
	}
}

// resumeAppWithBank wires the status surface over a stub bank, so the one behaviour that
// changed can be exercised: where the served work history comes from.
func resumeAppWithBank(t *testing.T, store *resume.Store, bank experienceBank) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &resumeHandlers{resume: store, bank: bank}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/me/resume", auth.RequireAuth(iss, testVersions), h.GetResume)
	return app, token
}

// The staleness rule used to hide everything: a newer CV whose extraction had not landed
// left the profile with no parsed résumé at all. The bank does not answer to that stamp,
// so the career survives the window while the file-owned sections wait for the extract.
func TestGetResumeServesBankedExperienceThroughAStaleWindow(t *testing.T) {
	store := resume.New(newFakeResumeBlobs(), &fakeResumeRepo{set: true}) // a stored CV, no current structure
	bank := &fakeBank{history: []resumeextract.Experience{
		{Company: "RingCentral", Title: "Senior Software Engineer", Highlights: []string{"Cut latency 20s to 1s"}},
	}}
	app, token := resumeAppWithBank(t, store, bank)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/me/resume", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		Data struct {
			Structured       *resumeextract.Structured `json:"structured"`
			StructurePending bool                      `json:"structure_pending"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Structured == nil {
		t.Fatal("structured is null though the bank holds a career")
	}
	if len(body.Data.Structured.Experience) != 1 || body.Data.Structured.Experience[0].Company != "RingCentral" {
		t.Errorf("experience = %+v, want the banked role", body.Data.Structured.Experience)
	}
	if !body.Data.StructurePending {
		t.Error("structure_pending = false, want true while the stamp is absent")
	}
}

func TestGetResumeProvisionalContactsThroughAStaleWindow(t *testing.T) {
	blob, _ := json.Marshal(resumeextract.Structured{
		FullName: "Ada Lovelace", Email: "ada@example.com", Phone: "+351 900",
		Links: []string{"https://github.com/ada"}, Summary: "should not appear",
	})
	// structAt behind resumeUploadedAt → stale; blob still has contacts.
	repo := &fakeResumeRepo{
		key: "resumes/1", set: true, structured: blob, structModel: "m",
		structAt: pgtype.Timestamptz{Time: resumeUploadedAt.Add(-time.Hour), Valid: true},
	}
	store := resume.New(newFakeResumeBlobs(), repo)
	bank := &fakeBank{history: []resumeextract.Experience{{Company: "RingCentral", Title: "SWE"}}}
	app, token := resumeAppWithBank(t, store, bank)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/me/resume", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		Data resumeMetaResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Data.StructurePending {
		t.Fatal("structure_pending = false, want true")
	}
	st := body.Data.Structured
	if st == nil {
		t.Fatal("structured is null")
	}
	if st.FullName != "Ada Lovelace" || st.Phone != "+351 900" || len(st.Links) != 1 {
		t.Errorf("contacts = %+v, want provisional identity", st)
	}
	if st.Summary != "" {
		t.Errorf("summary = %q, want empty while pending", st.Summary)
	}
	if len(st.Experience) != 1 || st.Experience[0].Company != "RingCentral" {
		t.Errorf("experience = %+v, want banked role", st.Experience)
	}
}

func TestGetResumeOwnedContactsOverlayCurrentExtract(t *testing.T) {
	blob, _ := json.Marshal(resumeextract.Structured{
		FullName: "From Blob", Email: "blob@example.com", Summary: "Staff engineer",
	})
	owned, _ := json.Marshal(resume.Owned{FullName: "Ada Lovelace", Email: "ada@example.com"})
	repo := &fakeResumeRepo{
		key: "resumes/1", set: true, structured: blob, structModel: "m",
		structAt: pgtype.Timestamptz{Time: resumeUploadedAt, Valid: true},
		contacts: owned,
	}
	store := resume.New(newFakeResumeBlobs(), repo)
	app, token := resumeAppWithBank(t, store, &fakeBank{})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/me/resume", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		Data resumeMetaResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	st := body.Data.Structured
	if st == nil {
		t.Fatal("structured is null")
	}
	if st.FullName != "Ada Lovelace" || st.Email != "ada@example.com" {
		t.Errorf("contacts = %+v, want owned block", st)
	}
	if st.Summary != "Staff engineer" {
		t.Errorf("summary = %q, want current extract body kept", st.Summary)
	}
	if body.Data.Contacts == nil || body.Data.Contacts.FullName != "Ada Lovelace" {
		t.Errorf("contacts field = %+v", body.Data.Contacts)
	}
}

func TestGetResumeCurrentStructureKeepsSemanticSections(t *testing.T) {
	blob, _ := json.Marshal(resumeextract.Structured{
		FullName: "Ada Lovelace", Summary: "Staff engineer", TotalYears: 11,
		Education: []resumeextract.Education{{Degree: "BSc"}},
	})
	repo := &fakeResumeRepo{
		key: "resumes/1", set: true, structured: blob, structModel: "m",
		structAt: pgtype.Timestamptz{Time: resumeUploadedAt, Valid: true},
	}
	store := resume.New(newFakeResumeBlobs(), repo)
	app, token := resumeAppWithBank(t, store, &fakeBank{})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/me/resume", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		Data resumeMetaResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.StructurePending {
		t.Fatal("structure_pending = true, want false for a current stamp")
	}
	st := body.Data.Structured
	if st == nil || st.Summary != "Staff engineer" || st.TotalYears != 11 || len(st.Education) != 1 {
		t.Fatalf("current structured = %+v", st)
	}
}

func TestGetResumeServesBankProjects(t *testing.T) {
	store := resume.New(newFakeResumeBlobs(), &fakeResumeRepo{set: true})
	bank := &fakeBank{
		history: []resumeextract.Experience{
			{Company: "RingCentral", Title: "SWE", Highlights: []string{"Shipped X"}},
		},
		projects: []resumeextract.Project{
			{Name: "opensched", Link: "https://opensched.dev", Highlights: []string{"1.4M channels"}},
		},
	}
	app, token := resumeAppWithBank(t, store, bank)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/me/resume", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		Data resumeMetaResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	st := body.Data.Structured
	if st == nil {
		t.Fatal("structured is null")
	}
	if len(st.Experience) != 1 || st.Experience[0].Company != "RingCentral" {
		t.Errorf("experience = %+v, want the banked job", st.Experience)
	}
	if len(st.Projects) != 1 || st.Projects[0].Name != "opensched" || st.Projects[0].Link != "https://opensched.dev" {
		t.Errorf("projects = %+v, want banked opensched with link", st.Projects)
	}
}

func TestGetResumeServesPlacelessHighlights(t *testing.T) {
	store := resume.New(newFakeResumeBlobs(), &fakeResumeRepo{set: true})
	bank := &fakeBank{
		history: []resumeextract.Experience{
			{Company: "RingCentral", Title: "SWE"},
			{Highlights: []string{"Built a Portuguese localization mod"}},
		},
	}
	app, token := resumeAppWithBank(t, store, bank)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/me/resume", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		Data resumeMetaResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	st := body.Data.Structured
	if st == nil || len(st.Experience) != 2 {
		t.Fatalf("experience = %+v, want job + placeless", st)
	}
	last := st.Experience[1]
	if last.Company != "" || last.Title != "" {
		t.Errorf("placeless place = company=%q title=%q, want empty", last.Company, last.Title)
	}
	if len(last.Highlights) != 1 || last.Highlights[0] != "Built a Portuguese localization mod" {
		t.Errorf("placeless highlights = %q", last.Highlights)
	}
}
