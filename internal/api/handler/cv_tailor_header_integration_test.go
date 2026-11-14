//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// TestTailorCVBootstrap_ReseedsStaleBaseAfterUpload: base predates a newer résumé
// upload → first tailor for a new vacancy refreshes base + tailored from the seed.
func TestTailorCVBootstrap_ReseedsStaleBaseAfterUpload(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	user := seedSubscriber(t, pool, "stale-base@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)

	// Old seed stamp + base content that predates the later upload.
	oldAt := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
	oldBlob, _ := json.Marshal(resumeextract.Structured{FullName: "Old Name", Summary: "before upload", Skills: []string{"Go"}})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2,
		 resume_structured = $3, resume_structured_uploaded_at = $2, resume_structured_model = 'test'
		 WHERE id = $1`, user, oldAt, oldBlob); err != nil {
		t.Fatalf("seed old résumé: %v", err)
	}
	base, err := h.cvStore.Create(ctx, user, "My CV", cv.DefaultTemplateID, cv.Document{
		Header:  cv.Header{FullName: "Old Name"},
		Summary: "before upload",
	})
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE cvs SET updated_at = $2 WHERE id = $1`, base.ID, oldAt); err != nil {
		t.Fatalf("backdate base: %v", err)
	}

	// Newer upload + structure (seed source moves; cvs intentionally untouched).
	newAt := time.Now().Truncate(time.Microsecond)
	newBlob, _ := json.Marshal(resumeextract.Structured{FullName: "Ada Lovelace", Summary: "after upload", Skills: []string{"Go", "Kafka"}})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_uploaded_at = $2,
		 resume_structured = $3, resume_structured_uploaded_at = $2 WHERE id = $1`,
		user, newAt, newBlob); err != nil {
		t.Fatalf("seed new résumé: %v", err)
	}

	jobID := seedJobSlug(t, pool, "stale-base-job")
	seedAnalysis(t, h, user, jobID)

	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "stale-base-job"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("bootstrap = %d, want 201", resp.StatusCode)
	}
	var got struct {
		Data tailorCVResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()

	tailored, err := h.cvStore.Get(ctx, mustParseUUID(t, got.Data.TailorCVID), user)
	if err != nil {
		t.Fatalf("get tailored: %v", err)
	}
	if tailored.Document.Summary != "after upload" || tailored.Document.Header.FullName != "Ada Lovelace" {
		t.Fatalf("tailored = %+v / %q, want after-upload seed", tailored.Document.Header, tailored.Document.Summary)
	}
	refreshed, ok, err := h.cvStore.BaseCV(ctx, user)
	if err != nil || !ok {
		t.Fatalf("BaseCV: ok=%v err=%v", ok, err)
	}
	if refreshed.ID != base.ID {
		t.Fatalf("base id changed: %s → %s", base.ID, refreshed.ID)
	}
	if refreshed.Document.Summary != "after upload" {
		t.Fatalf("base summary = %q, want after upload", refreshed.Document.Summary)
	}

	// Idempotent reload keeps the same tailored id and does not wipe content.
	resp2 := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "stale-base-job"})
	if resp2.StatusCode != fiber.StatusCreated {
		t.Fatalf("reload = %d", resp2.StatusCode)
	}
	var got2 struct {
		Data tailorCVResponse `json:"data"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&got2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp2.Body.Close()
	if got2.Data.TailorCVID != got.Data.TailorCVID {
		t.Fatalf("reload minted new tailored CV")
	}
}

// Pending extraction after a re-upload must not blank a filled base header: the bank may
// have roles, but without a current structured résumé the seed is not usable for replace.
func TestTailorCVBootstrap_PendingStructureDoesNotBlankHeader(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	user := seedSubscriber(t, pool, "pending-struct@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)

	oldAt := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
	base, err := h.cvStore.Create(ctx, user, "My CV", cv.DefaultTemplateID, cv.Document{
		Header: cv.Header{
			FullName: "Ada Lovelace",
			Email:    "ada@example.com",
			Phone:    "+351 900 000 000",
			Location: "Lisbon, PT",
			Links:    []string{"github.com/ada"},
		},
		Summary: "keep me",
	})
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE cvs SET updated_at = $2 WHERE id = $1`, base.ID, oldAt); err != nil {
		t.Fatalf("backdate base: %v", err)
	}

	// Newer upload, but no current structure (extraction pending / stamp mismatch).
	newAt := time.Now().Truncate(time.Microsecond)
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2,
		 resume_structured = NULL, resume_structured_uploaded_at = NULL WHERE id = $1`,
		user, newAt); err != nil {
		t.Fatalf("seed pending résumé: %v", err)
	}
	seedBankedCareer(t, h.queries, user)

	jobID := seedJobSlug(t, pool, "pending-struct-job")
	seedAnalysis(t, h, user, jobID)

	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "pending-struct-job"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("bootstrap = %d, want 201", resp.StatusCode)
	}
	resp.Body.Close()

	refreshed, ok, err := h.cvStore.BaseCV(ctx, user)
	if err != nil || !ok {
		t.Fatalf("BaseCV: ok=%v err=%v", ok, err)
	}
	want := cv.Header{
		FullName: "Ada Lovelace", Email: "ada@example.com", Phone: "+351 900 000 000",
		Location: "Lisbon, PT", Links: []string{"github.com/ada"},
	}
	if refreshed.Document.Header.FullName != want.FullName ||
		refreshed.Document.Header.Email != want.Email ||
		refreshed.Document.Header.Phone != want.Phone ||
		refreshed.Document.Header.Location != want.Location ||
		len(refreshed.Document.Header.Links) != 1 || refreshed.Document.Header.Links[0] != want.Links[0] {
		t.Fatalf("base header blanked: %+v, want %+v", refreshed.Document.Header, want)
	}
	if refreshed.Document.Summary != "keep me" {
		t.Fatalf("base summary = %q, want keep me", refreshed.Document.Summary)
	}
}

// A newer, CURRENT structured extract that itself carries identity alone (e.g. an
// extraction that only recovered a name) must not blank the base's existing body either —
// "current" is not the same as "has body content". Mirrors the pending-structure case
// above but for the "current, yet bodyless" seed hasSeedBody exists to catch.
func TestTailorCVBootstrap_IdentityOnlyCurrentStructureDoesNotBlankBase(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	user := seedSubscriber(t, pool, "identity-only-current@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)

	oldAt := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
	base, err := h.cvStore.Create(ctx, user, "My CV", cv.DefaultTemplateID, cv.Document{
		Header:     cv.Header{FullName: "Old Base Name"},
		Summary:    "hand-written summary I typed myself",
		Experience: []cv.ExperienceItem{{Company: "Real Job I Had"}},
	})
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE cvs SET updated_at = $2 WHERE id = $1`, base.ID, oldAt); err != nil {
		t.Fatalf("backdate base: %v", err)
	}

	newAt := time.Now().Truncate(time.Microsecond)
	newBlob, _ := json.Marshal(resumeextract.Structured{FullName: "Ada Lovelace"})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2,
		 resume_structured = $3, resume_structured_uploaded_at = $2 WHERE id = $1`,
		user, newAt, newBlob); err != nil {
		t.Fatalf("seed identity-only current résumé: %v", err)
	}

	jobID := seedJobSlug(t, pool, "identity-only-current-job")
	seedAnalysis(t, h, user, jobID)

	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "identity-only-current-job"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("bootstrap = %d, want 201", resp.StatusCode)
	}
	resp.Body.Close()

	refreshed, ok, err := h.cvStore.BaseCV(ctx, user)
	if err != nil || !ok {
		t.Fatalf("BaseCV: ok=%v err=%v", ok, err)
	}
	if refreshed.Document.Summary != "hand-written summary I typed myself" || len(refreshed.Document.Experience) != 1 {
		t.Fatalf("base body blanked by identity-only reseed: summary=%q experience=%+v",
			refreshed.Document.Summary, refreshed.Document.Experience)
	}
	if refreshed.Document.Header.FullName != "Old Base Name" {
		t.Fatalf("header = %+v, want existing name kept (heal is keep-first)", refreshed.Document.Header)
	}
}

func TestTailorCVBootstrap_KeepsBaseEditedAfterUpload(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	user := seedSubscriber(t, pool, "edited-base@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)

	uploadAt := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	blob, _ := json.Marshal(resumeextract.Structured{FullName: "From Seed", Summary: "from seed", Skills: []string{"Go"}})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2,
		 resume_structured = $3, resume_structured_uploaded_at = $2 WHERE id = $1`,
		user, uploadAt, blob); err != nil {
		t.Fatalf("seed résumé: %v", err)
	}

	base, err := h.cvStore.Create(ctx, user, "My CV", cv.DefaultTemplateID, cv.Document{
		Header:  cv.Header{FullName: "Hand Edited"},
		Summary: "hand edited",
	})
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	// Ensure updated_at is after the upload stamp.
	if _, err := pool.Exec(ctx, `UPDATE cvs SET updated_at = $2 WHERE id = $1`,
		base.ID, uploadAt.Add(time.Minute)); err != nil {
		t.Fatalf("stamp base: %v", err)
	}

	jobID := seedJobSlug(t, pool, "edited-base-job")
	seedAnalysis(t, h, user, jobID)

	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "edited-base-job"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("bootstrap = %d, want 201", resp.StatusCode)
	}
	var got struct {
		Data tailorCVResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()

	tailored, err := h.cvStore.Get(ctx, mustParseUUID(t, got.Data.TailorCVID), user)
	if err != nil {
		t.Fatalf("get tailored: %v", err)
	}
	if tailored.Document.Summary != "hand edited" {
		t.Fatalf("tailored summary = %q, want hand edited (no forced reseed)", tailored.Document.Summary)
	}
}

// Empty base + stale structure with contacts + bank: tailor heals the header without
// wiping a hand-written summary (provisional path is header-only on reseed).
func TestTailorCVBootstrap_ProvisionalContactsFillEmptyHeader(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	user := seedSubscriber(t, pool, "provisional-header@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)

	oldAt := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
	oldBlob, _ := json.Marshal(resumeextract.Structured{
		FullName: "Ada Lovelace", Email: "ada@example.com", Phone: "+44",
		Summary: "stale summary", Skills: []string{"Go"},
	})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2,
		 resume_structured = $3, resume_structured_uploaded_at = $2, resume_structured_model = 'test'
		 WHERE id = $1`, user, oldAt, oldBlob); err != nil {
		t.Fatalf("seed old structure: %v", err)
	}
	base, err := h.cvStore.Create(ctx, user, "My CV", cv.DefaultTemplateID, cv.Document{
		Header:  cv.Header{},
		Summary: "keep me",
	})
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE cvs SET updated_at = $2 WHERE id = $1`, base.ID, oldAt); err != nil {
		t.Fatalf("backdate base: %v", err)
	}
	newAt := time.Now().Truncate(time.Microsecond)
	if _, err := pool.Exec(ctx, `UPDATE users SET resume_uploaded_at = $2 WHERE id = $1`, user, newAt); err != nil {
		t.Fatalf("newer upload stamp: %v", err)
	}
	seedBankedCareer(t, h.queries, user)

	jobID := seedJobSlug(t, pool, "provisional-header-job")
	seedAnalysis(t, h, user, jobID)

	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/tailor", tok, tailorCVRequest{JobSlug: "provisional-header-job"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("bootstrap = %d, want 201", resp.StatusCode)
	}
	var got struct {
		Data tailorCVResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()

	refreshed, ok, err := h.cvStore.BaseCV(ctx, user)
	if err != nil || !ok {
		t.Fatalf("BaseCV: ok=%v err=%v", ok, err)
	}
	if refreshed.Document.Header.FullName != "Ada Lovelace" || refreshed.Document.Header.Email != "ada@example.com" {
		t.Fatalf("base header = %+v, want provisional contacts", refreshed.Document.Header)
	}
	if refreshed.Document.Summary != "keep me" {
		t.Fatalf("base summary = %q, want keep me (provisional reseed is header-only)", refreshed.Document.Summary)
	}

	tailored, err := h.cvStore.Get(ctx, mustParseUUID(t, got.Data.TailorCVID), user)
	if err != nil {
		t.Fatalf("get tailored: %v", err)
	}
	if tailored.Document.Header.FullName != "Ada Lovelace" {
		t.Fatalf("tailored header = %+v, want healed name", tailored.Document.Header)
	}
}

func TestGetCV_HealsEmptyTailoredHeaderFromProvisional(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	user := seedSubscriber(t, pool, "heal-get@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)

	oldAt := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	blob, _ := json.Marshal(resumeextract.Structured{
		FullName: "Ada Lovelace", Email: "ada@example.com",
	})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2,
		 resume_structured = $3, resume_structured_uploaded_at = $2 WHERE id = $1`,
		user, oldAt, blob); err != nil {
		t.Fatalf("seed structure: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET resume_uploaded_at = $2 WHERE id = $1`,
		user, oldAt.Add(30*time.Minute)); err != nil {
		t.Fatalf("stale stamp: %v", err)
	}

	jobID := seedJobSlug(t, pool, "heal-get-job")
	tailored, err := h.cvStore.CreateTailored(ctx, user, jobID, "T", cv.DefaultTemplateID, cv.Document{
		Header: cv.Header{}, Summary: "body",
	})
	if err != nil {
		t.Fatalf("create tailored: %v", err)
	}

	resp := doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+tailored.ID.String(), tok, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("GET = %d", resp.StatusCode)
	}
	var body struct {
		Data struct {
			Document cv.Document `json:"document"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if body.Data.Document.Header.FullName != "Ada Lovelace" || body.Data.Document.Header.Email != "ada@example.com" {
		t.Fatalf("response header = %+v", body.Data.Document.Header)
	}
	if body.Data.Document.Summary != "body" {
		t.Fatalf("summary = %q, want body unchanged", body.Data.Document.Summary)
	}

	stored, err := h.cvStore.Get(ctx, tailored.ID, user)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Document.Header.FullName != "Ada Lovelace" {
		t.Fatalf("stored header not persisted: %+v", stored.Document.Header)
	}
}

func TestGetCV_SecondHealDoesNotWriteRevision(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	user := seedSubscriber(t, pool, "heal-once@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)

	oldAt := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	blob, _ := json.Marshal(resumeextract.Structured{
		FullName: "Ada Lovelace", Email: "ada@example.com",
	})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2,
		 resume_structured = $3, resume_structured_uploaded_at = $2 WHERE id = $1`,
		user, oldAt, blob); err != nil {
		t.Fatalf("seed structure: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET resume_uploaded_at = $2 WHERE id = $1`,
		user, oldAt.Add(30*time.Minute)); err != nil {
		t.Fatalf("stale stamp: %v", err)
	}

	jobID := seedJobSlug(t, pool, "heal-once-job")
	tailored, err := h.cvStore.CreateTailored(ctx, user, jobID, "T", cv.DefaultTemplateID, cv.Document{
		Header: cv.Header{}, Summary: "body",
	})
	if err != nil {
		t.Fatalf("create tailored: %v", err)
	}

	first := doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+tailored.ID.String(), tok, nil)
	if first.StatusCode != fiber.StatusOK {
		t.Fatalf("first GET = %d", first.StatusCode)
	}
	first.Body.Close()

	var afterFirst int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM cv_revisions WHERE cv_id = $1`, tailored.ID).Scan(&afterFirst); err != nil {
		t.Fatalf("count after first: %v", err)
	}
	if afterFirst < 1 {
		t.Fatalf("revisions after first GET = %d, want at least the heal", afterFirst)
	}

	second := doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+tailored.ID.String(), tok, nil)
	if second.StatusCode != fiber.StatusOK {
		t.Fatalf("second GET = %d", second.StatusCode)
	}
	second.Body.Close()

	var afterSecond int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM cv_revisions WHERE cv_id = $1`, tailored.ID).Scan(&afterSecond); err != nil {
		t.Fatalf("count after second: %v", err)
	}
	if afterSecond != afterFirst {
		t.Fatalf("revisions after second GET = %d, want %d (no new revision)", afterSecond, afterFirst)
	}
}

func TestListCVsAndPDFDoNotHealHeader(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	h.cvRenderer = &fakeCVRenderer{pdf: []byte("%PDF-1.4 fake")}
	h.pdf = ownedCVPDF{store: h.cvStore, renderer: h.cvRenderer, photos: h.photos}
	app := buildTailorApp(h, iss)
	saved := auth.RequireAuth(iss, testVersions)
	app.Get("/api/v1/me/cvs", saved, cvContract(h))
	app.Get("/api/v1/me/cvs/:id/pdf", saved, h.RenderCVPDF)
	ctx := context.Background()

	user := seedSubscriber(t, pool, "heal-list-pdf@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)

	oldAt := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	blob, _ := json.Marshal(resumeextract.Structured{FullName: "Ada Lovelace"})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2,
		 resume_structured = $3, resume_structured_uploaded_at = $2 WHERE id = $1`,
		user, oldAt, blob); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET resume_uploaded_at = $2 WHERE id = $1`,
		user, oldAt.Add(30*time.Minute)); err != nil {
		t.Fatalf("stale: %v", err)
	}

	jobID := seedJobSlug(t, pool, "heal-list-pdf-job")
	tailored, err := h.cvStore.CreateTailored(ctx, user, jobID, "T", cv.DefaultTemplateID, cv.Document{
		Header: cv.Header{}, Summary: "body",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	list := doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs", tok, nil)
	if list.StatusCode != fiber.StatusOK {
		t.Fatalf("LIST = %d", list.StatusCode)
	}
	list.Body.Close()

	afterList, err := h.cvStore.Get(ctx, tailored.ID, user)
	if err != nil {
		t.Fatalf("get after list: %v", err)
	}
	if afterList.Document.Header.FullName != "" {
		t.Fatalf("list healed header: %+v", afterList.Document.Header)
	}

	pdf := doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+tailored.ID.String()+"/pdf", tok, nil)
	if pdf.StatusCode != fiber.StatusOK {
		t.Fatalf("PDF = %d", pdf.StatusCode)
	}
	pdf.Body.Close()

	afterPDF, err := h.cvStore.Get(ctx, tailored.ID, user)
	if err != nil {
		t.Fatalf("get after pdf: %v", err)
	}
	if afterPDF.Document.Header.FullName != "" {
		t.Fatalf("pdf healed header: %+v", afterPDF.Document.Header)
	}
}

func TestGetCV_PartialHeaderKeepsNameFillsEmail(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	app := buildTailorApp(h, iss)
	ctx := context.Background()

	user := seedSubscriber(t, pool, "heal-partial@example.test", true)
	tok, _ := iss.Issue(user, testTokenVersion)

	oldAt := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	blob, _ := json.Marshal(resumeextract.Structured{
		FullName: "From Blob", Email: "blob@example.com",
	})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2,
		 resume_structured = $3, resume_structured_uploaded_at = $2 WHERE id = $1`,
		user, oldAt, blob); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET resume_uploaded_at = $2 WHERE id = $1`,
		user, oldAt.Add(time.Hour)); err != nil {
		t.Fatalf("stale: %v", err)
	}

	jobID := seedJobSlug(t, pool, "heal-partial-job")
	tailored, err := h.cvStore.CreateTailored(ctx, user, jobID, "T", cv.DefaultTemplateID, cv.Document{
		Header: cv.Header{FullName: "Keep Me"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	resp := doCV(t, app, fiber.MethodGet, "/api/v1/me/cvs/"+tailored.ID.String(), tok, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("GET = %d", resp.StatusCode)
	}
	var body struct {
		Data struct {
			Document cv.Document `json:"document"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if body.Data.Document.Header.FullName != "Keep Me" {
		t.Fatalf("name = %q, want Keep Me", body.Data.Document.Header.FullName)
	}
	if body.Data.Document.Header.Email != "blob@example.com" {
		t.Fatalf("email = %q, want filled from provisional", body.Data.Document.Header.Email)
	}
}
