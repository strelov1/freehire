//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
)

func TestReseedBaseCV_CreatesBaseWhenAbsent(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	userID := seedAccount(t, pool, "nobase@example.com", false)
	tok, _ := iss.Issue(userID, 1)
	ctx := context.Background()
	blob, _ := json.Marshal(resumeextract.Structured{FullName: "New Ada", Summary: "from seed"})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = now(),
		 resume_structured = $2, resume_structured_uploaded_at = now() WHERE id = $1`,
		userID, blob); err != nil {
		t.Fatalf("seed structured: %v", err)
	}
	app := buildResetApp(h, iss)
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/base/reseed", tok, nil)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	base, ok, err := h.cvStore.BaseCV(ctx, userID)
	if err != nil || !ok {
		t.Fatalf("expected a base CV after reset: ok=%v err=%v", ok, err)
	}
	if base.Document.Header.FullName != "New Ada" {
		t.Fatalf("new base FullName = %q", base.Document.Header.FullName)
	}
	// No saved appearance defaults for this user: the new base must still get the system's
	// hardcoded defaults, exactly as before the add-cv-appearance-defaults change.
	if base.TemplateID != cv.DefaultTemplateID {
		t.Fatalf("new base template = %q, want system default %q", base.TemplateID, cv.DefaultTemplateID)
	}
	if base.Document.Margins != cv.DefaultMargins() {
		t.Fatalf("new base margins = %+v, want system default %+v", base.Document.Margins, cv.DefaultMargins())
	}
}

// When the user has no base CV yet, reseedBaseFromSeed's create branch (cv_reseed.go) must
// start the new base from the user's saved appearance defaults — see the
// add-cv-appearance-defaults change.
func TestReseedBaseCV_CreatesBaseFromSavedAppearanceDefaults(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	userID := seedAccount(t, pool, "nobase-defaults@example.com", false)
	tok, _ := iss.Issue(userID, 1)
	ctx := context.Background()
	blob, _ := json.Marshal(resumeextract.Structured{FullName: "New Ada", Summary: "from seed"})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = now(),
		 resume_structured = $2, resume_structured_uploaded_at = now() WHERE id = $1`,
		userID, blob); err != nil {
		t.Fatalf("seed structured: %v", err)
	}
	saved := cv.AppearanceDefaults{
		TemplateID: "timeline",
		Style:      cv.Style{FontFamily: "carlito", FontSize: 10, LineHeight: 0.65},
		Margins:    cv.Margins{Top: 1, Right: 1, Bottom: 1, Left: 1},
	}
	if _, err := h.cvStore.SetAppearanceDefaults(ctx, userID, saved); err != nil {
		t.Fatalf("set appearance defaults: %v", err)
	}
	app := buildResetApp(h, iss)
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/base/reseed", tok, nil)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	base, ok, err := h.cvStore.BaseCV(ctx, userID)
	if err != nil || !ok {
		t.Fatalf("expected a base CV after reset: ok=%v err=%v", ok, err)
	}
	if base.TemplateID != saved.TemplateID {
		t.Fatalf("new base template = %q, want saved default %q", base.TemplateID, saved.TemplateID)
	}
	if base.Document.Style != saved.Style {
		t.Fatalf("new base style = %+v, want saved default %+v", base.Document.Style, saved.Style)
	}
	if base.Document.Margins != saved.Margins {
		t.Fatalf("new base margins = %+v, want saved default %+v", base.Document.Margins, saved.Margins)
	}
}

// Pointed at a base, the reseed is the candidate's explicit "update from my CV": it replaces
// THAT base (even one they edited by hand) and nothing else.
func TestReseedCV_BaseTargetReplacesOnlyThatBase(t *testing.T) {
	f := newResetFixture(t)
	other, err := f.store.Create(context.Background(), f.userID, "Second", cv.DefaultTemplateID, cv.Document{Summary: "second base"})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/me/cvs/" + other.ID.String() + "/reseed"
	resp := doCV(t, f.app, fiber.MethodPost, path, f.token, nil)
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}
	var out struct {
		Data cvResponse `json:"data"`
	}
	decodeJSON(t, resp, &out)
	if out.Data.ID != other.ID.String() || out.Data.Document.Summary != "Résumé seed summary" {
		t.Fatalf("reseeded base = %s / %q, want the named base refreshed from the seed", out.Data.ID, out.Data.Document.Summary)
	}
	first, err := f.store.Get(context.Background(), f.base.ID, f.userID)
	if err != nil || first.Document.Summary != "old base summary" {
		t.Fatalf("the other base was touched: %+v err=%v", first.Document.Summary, err)
	}
	tailored, err := f.store.Get(context.Background(), f.tailored.ID, f.userID)
	if err != nil || tailored.Document.Summary != "old tailored summary" {
		t.Fatalf("a tailored copy was touched: %q err=%v", tailored.Document.Summary, err)
	}
}

func TestReseedBaseCV_HappyPath(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	userID := seedAccount(t, pool, "base-reset@example.com", false)
	tok, _ := iss.Issue(userID, 1)
	ctx := context.Background()
	blob, _ := json.Marshal(resumeextract.Structured{
		FullName: "Ada Lovelace", Email: "ada@example.com", Summary: "from seed",
	})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = now(),
		 resume_structured = $2, resume_structured_uploaded_at = now(),
		 resume_structured_model = 'test' WHERE id = $1`,
		userID, blob); err != nil {
		t.Fatalf("seed structured: %v", err)
	}
	seedBankedCareer(t, h.queries, userID)

	store := h.cvStore
	base, err := store.Create(ctx, userID, "My CV", cv.DefaultTemplateID, cv.Document{
		Margins: cv.Margins{Top: 0.75, Right: 0.75, Bottom: 0.75, Left: 0.75},
		Style:   cv.Style{FontSize: 11},
		Header:  cv.Header{FullName: "Old Base"},
		Summary: "old base summary",
		Experience: []cv.ExperienceItem{{
			Role: "Stale", Company: "OldCo",
		}},
	})
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	jobID := seedJobSlug(t, pool, "base-reset-"+uuid.NewString()[:8])
	tailored, err := store.CreateTailored(ctx, userID, jobID, "Tailored", cv.DefaultTemplateID, cv.Document{
		Header:  cv.Header{FullName: "Old Tailored"},
		Summary: "old tailored summary",
	})
	if err != nil {
		t.Fatalf("create tailored: %v", err)
	}

	app := buildResetApp(h, iss)
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/base/reseed", tok, nil)
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var out struct {
		Data cvResponse `json:"data"`
	}
	decodeJSON(t, resp, &out)
	if out.Data.ID != base.ID.String() {
		t.Fatalf("id = %s, want base %s", out.Data.ID, base.ID)
	}
	if out.Data.Document.Header.FullName != "Ada Lovelace" || out.Data.Document.Summary != "from seed" {
		t.Fatalf("base content = %+v / %q, want seed", out.Data.Document.Header, out.Data.Document.Summary)
	}
	if out.Data.Document.Margins.Top != 0.75 || out.Data.Document.Style.FontSize != 11 {
		t.Fatalf("presentation clobbered: margins=%+v style=%+v", out.Data.Document.Margins, out.Data.Document.Style)
	}
	if len(out.Data.Document.Experience) == 0 || out.Data.Document.Experience[0].Company != "Acme" {
		t.Fatalf("experience = %+v, want banked Acme", out.Data.Document.Experience)
	}

	gotTailored, err := store.Get(ctx, tailored.ID, userID)
	if err != nil {
		t.Fatalf("get tailored: %v", err)
	}
	if gotTailored.Document.Summary != "old tailored summary" {
		t.Fatalf("tailored summary = %q, want untouched", gotTailored.Document.Summary)
	}
}

func TestReseedBaseCV_NoSeed409(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	userID := seedAccount(t, pool, "base-noseed@example.com", false)
	tok, _ := iss.Issue(userID, 1)
	ctx := context.Background()
	if _, err := h.cvStore.Create(ctx, userID, "My CV", cv.DefaultTemplateID, cv.Document{
		Summary: "keep me",
	}); err != nil {
		t.Fatalf("create base: %v", err)
	}
	app := buildResetApp(h, iss)
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/base/reseed", tok, nil)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s, want 409", resp.StatusCode, body)
	}
	base, ok, err := h.cvStore.BaseCV(ctx, userID)
	if err != nil || !ok {
		t.Fatalf("BaseCV: ok=%v err=%v", ok, err)
	}
	if base.Document.Summary != "keep me" {
		t.Fatalf("summary = %q, want untouched on 409", base.Document.Summary)
	}
}

func TestReseedBaseCV_OtherUserLeavesOwnersBase(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	owner := seedAccount(t, pool, "base-owner@example.com", false)
	other := seedAccount(t, pool, "base-other@example.com", false)
	otherTok, _ := iss.Issue(other, 1)
	ctx := context.Background()
	if _, err := h.cvStore.Create(ctx, owner, "My CV", cv.DefaultTemplateID, cv.Document{
		Summary: "owner keep me",
	}); err != nil {
		t.Fatalf("create owner base: %v", err)
	}
	app := buildResetApp(h, iss)
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/base/reseed", otherTok, nil)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s, want 409 (other has no seed)", resp.StatusCode, body)
	}
	base, ok, err := h.cvStore.BaseCV(ctx, owner)
	if err != nil || !ok {
		t.Fatalf("owner BaseCV: ok=%v err=%v", ok, err)
	}
	if base.Document.Summary != "owner keep me" {
		t.Fatalf("owner summary = %q, want untouched by another account", base.Document.Summary)
	}
}
