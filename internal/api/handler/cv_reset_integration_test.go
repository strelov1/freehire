//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/identity/auth"
)

func buildResetApp(h *cvHandlers, iss *auth.Issuer) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	saved := auth.RequireAuth(iss, testVersions)
	keyAuth := auth.RequireAuthOrScopedKey(iss, testVersions, apiKeys{h.queries}, auth.ScopeCV)
	app.Get("/api/v1/me/cvs/:id", keyAuth, h.GetCV)
	app.Post("/api/v1/me/cvs/base/reset-from-resume", saved, h.ResetBaseCVFromResume)
	app.Post("/api/v1/me/cvs/:id/reset-from-resume", saved, h.ResetCVFromResume)
	return app
}

type resetFixture struct {
	h        *cvHandlers
	app      *fiber.App
	iss      *auth.Issuer
	pool     *pgxpool.Pool
	token    string
	userID   int64
	base     cv.Meta
	tailored cv.Meta
	store    *cv.Store
}

func newResetFixture(t *testing.T) resetFixture {
	t.Helper()
	h, iss, pool := newTailorAPI(t)
	userID := seedAccount(t, pool, "reset@example.com", false)
	tok, err := iss.Issue(userID, 1)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	ctx := context.Background()
	blob, _ := json.Marshal(resumeextract.Structured{
		FullName: "Ada Lovelace",
		Email:    "ada@example.com",
		Summary:  "Résumé seed summary",
		Skills:   []string{"Go", "PostgreSQL"},
	})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = now(),
		 resume_structured = $2, resume_structured_uploaded_at = now(),
		 resume_structured_model = 'test' WHERE id = $1`,
		userID, blob); err != nil {
		t.Fatalf("seed structured: %v", err)
	}

	store := h.cvStore
	base, err := store.Create(ctx, userID, "My CV", cv.DefaultTemplateID, cv.Document{
		Margins: cv.Margins{Top: 0.75, Right: 0.75, Bottom: 0.75, Left: 0.75},
		Style:   cv.Style{FontSize: 11},
		Header:  cv.Header{FullName: "Old Base"},
		Summary: "old base summary",
	})
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	jobID := seedJobSlug(t, pool, "reset-job-"+uuid.NewString()[:8])
	tailored, err := store.CreateTailored(ctx, userID, jobID, "Tailored for Role", cv.DefaultTemplateID, cv.Document{
		Margins: cv.Margins{Top: 0.6, Right: 0.6, Bottom: 0.6, Left: 0.6},
		Style:   cv.Style{FontSize: 10.5, LineHeight: 0.5},
		Header:  cv.Header{FullName: "Old Tailored"},
		Summary: "old tailored summary",
	})
	if err != nil {
		t.Fatalf("create tailored: %v", err)
	}
	if err := store.SetSession(ctx, tailored.ID, userID, "sess-keep"); err != nil {
		t.Fatalf("set session: %v", err)
	}

	return resetFixture{
		h: h, app: buildResetApp(h, iss), iss: iss, pool: pool, token: tok,
		userID: userID, base: base, tailored: tailored, store: store,
	}
}

func TestResetCVFromResume_HappyPath(t *testing.T) {
	f := newResetFixture(t)
	path := "/api/v1/me/cvs/" + f.tailored.ID.String() + "/reset-from-resume"
	resp := doCV(t, f.app, fiber.MethodPost, path, f.token, nil)
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var out struct {
		Data cvResponse `json:"data"`
	}
	decodeJSON(t, resp, &out)
	if out.Data.ID != f.tailored.ID.String() {
		t.Fatalf("id = %s, want same tailored id %s", out.Data.ID, f.tailored.ID)
	}
	if out.Data.Document.Header.FullName != "Ada Lovelace" || out.Data.Document.Summary != "Résumé seed summary" {
		t.Fatalf("tailored content = %+v / %q, want résumé seed", out.Data.Document.Header, out.Data.Document.Summary)
	}
	if out.Data.Document.Margins.Top != 0.6 || out.Data.Document.Style.FontSize != 10.5 {
		t.Fatalf("tailored presentation clobbered: margins=%+v style=%+v", out.Data.Document.Margins, out.Data.Document.Style)
	}
	if out.Data.AgentSessionID != "sess-keep" {
		t.Fatalf("session = %q, want sess-keep", out.Data.AgentSessionID)
	}

	base, ok, err := f.store.BaseCV(context.Background(), f.userID)
	if err != nil || !ok {
		t.Fatalf("BaseCV: ok=%v err=%v", ok, err)
	}
	if base.ID != f.base.ID {
		t.Fatalf("base id changed: %s → %s", f.base.ID, base.ID)
	}
	if base.Document.Header.FullName != "Ada Lovelace" || base.Document.Summary != "Résumé seed summary" {
		t.Fatalf("base content = %+v / %q", base.Document.Header, base.Document.Summary)
	}
	if base.Document.Margins.Top != 0.75 || base.Document.Style.FontSize != 11 {
		t.Fatalf("base presentation clobbered: margins=%+v style=%+v", base.Document.Margins, base.Document.Style)
	}
}

func TestResetCVFromResume_CreatesBaseWhenAbsent(t *testing.T) {
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
	jobID := seedJobSlug(t, pool, "nobase-"+uuid.NewString()[:8])
	tailored, err := h.cvStore.CreateTailored(ctx, userID, jobID, "Only Tailored", cv.DefaultTemplateID, cv.Document{
		Summary: "stale",
	})
	if err != nil {
		t.Fatalf("create tailored: %v", err)
	}
	app := buildResetApp(h, iss)
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/"+tailored.ID.String()+"/reset-from-resume", tok, nil)
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

// When the user has no base CV yet, reseedBaseFromSeed's create branch (cv_reset.go) must
// start the new base from the user's saved appearance defaults — see the
// add-cv-appearance-defaults change.
func TestResetCVFromResume_CreatesBaseFromSavedAppearanceDefaults(t *testing.T) {
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
	jobID := seedJobSlug(t, pool, "nobase-defaults-"+uuid.NewString()[:8])
	tailored, err := h.cvStore.CreateTailored(ctx, userID, jobID, "Only Tailored", cv.DefaultTemplateID, cv.Document{
		Summary: "stale",
	})
	if err != nil {
		t.Fatalf("create tailored: %v", err)
	}
	app := buildResetApp(h, iss)
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/"+tailored.ID.String()+"/reset-from-resume", tok, nil)
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

func TestResetCVFromResume_BaseTarget409(t *testing.T) {
	f := newResetFixture(t)
	path := "/api/v1/me/cvs/" + f.base.ID.String() + "/reset-from-resume"
	resp := doCV(t, f.app, fiber.MethodPost, path, f.token, nil)
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
}

func TestResetCVFromResume_NoSeed409(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	userID := seedAccount(t, pool, "noseed@example.com", false)
	tok, _ := iss.Issue(userID, 1)
	ctx := context.Background()
	jobID := seedJobSlug(t, pool, "noseed-"+uuid.NewString()[:8])
	tailored, err := h.cvStore.CreateTailored(ctx, userID, jobID, "T", cv.DefaultTemplateID, cv.Document{Summary: "x"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	app := buildResetApp(h, iss)
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/"+tailored.ID.String()+"/reset-from-resume", tok, nil)
	if resp.StatusCode != fiber.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s, want 409", resp.StatusCode, body)
	}
}

// Banked experience without a current structured résumé is not a usable seed — reset must
// refuse rather than blank the tailored header.
func TestResetCVFromResume_BankOnlySeed409(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	userID := seedAccount(t, pool, "bankonly@example.com", false)
	tok, _ := iss.Issue(userID, 1)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = now(),
		 resume_structured = NULL, resume_structured_uploaded_at = NULL WHERE id = $1`,
		userID); err != nil {
		t.Fatalf("seed upload without structure: %v", err)
	}
	seedBankedCareer(t, h.queries, userID)

	jobID := seedJobSlug(t, pool, "bankonly-"+uuid.NewString()[:8])
	header := cv.Header{
		FullName: "Ada Lovelace", Email: "ada@example.com", Phone: "+351 900 000 000",
		Location: "Lisbon, PT", Links: []string{"github.com/ada"},
	}
	tailored, err := h.cvStore.CreateTailored(ctx, userID, jobID, "T", cv.DefaultTemplateID, cv.Document{
		Header: header, Summary: "keep me",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	app := buildResetApp(h, iss)
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/"+tailored.ID.String()+"/reset-from-resume", tok, nil)
	if resp.StatusCode != fiber.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s, want 409", resp.StatusCode, body)
	}
	resp.Body.Close()

	got, err := h.cvStore.Get(ctx, tailored.ID, userID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Document.Header.FullName != header.FullName || got.Document.Header.Email != header.Email ||
		got.Document.Header.Phone != header.Phone || got.Document.Header.Location != header.Location ||
		got.Document.Summary != "keep me" {
		t.Fatalf("tailored wiped: header=%+v summary=%q", got.Document.Header, got.Document.Summary)
	}
}

// A candidate who only set independent contact fields (no résumé ever uploaded, no bank
// rows) is identity-only. StructureForSeed/seedable both treat FullName alone as "usable"
// for first-time bootstrap, but Reset destructively replaces an EXISTING CV's whole body —
// identity alone must not be enough to wipe hand-written content.
func TestResetCVFromResume_IdentityOnlySeed409(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	userID := seedAccount(t, pool, "identityonly@example.com", false)
	tok, _ := iss.Issue(userID, 1)
	ctx := context.Background()

	contacts, _ := json.Marshal(map[string]string{
		"full_name": "Ada Lovelace",
		"email":     "ada@example.com",
	})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET candidate_contacts = $2 WHERE id = $1`,
		userID, contacts); err != nil {
		t.Fatalf("seed candidate contacts: %v", err)
	}

	jobID := seedJobSlug(t, pool, "identityonly-"+uuid.NewString()[:8])
	tailored, err := h.cvStore.CreateTailored(ctx, userID, jobID, "T", cv.DefaultTemplateID, cv.Document{
		Header:     cv.Header{FullName: "Hand-typed Name"},
		Summary:    "Hand-written summary I typed myself",
		Experience: []cv.ExperienceItem{{Company: "Real Job I Had"}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	app := buildResetApp(h, iss)
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/"+tailored.ID.String()+"/reset-from-resume", tok, nil)
	if resp.StatusCode != fiber.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s, want 409", resp.StatusCode, body)
	}
	resp.Body.Close()

	got, err := h.cvStore.Get(ctx, tailored.ID, userID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Document.Header.FullName != "Hand-typed Name" ||
		got.Document.Summary != "Hand-written summary I typed myself" ||
		len(got.Document.Experience) != 1 {
		t.Fatalf("tailored wiped: header=%+v summary=%q experience=%+v",
			got.Document.Header, got.Document.Summary, got.Document.Experience)
	}
}

func TestResetCVFromResume_ProvisionalContactsPlusBankSucceeds(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	userID := seedAccount(t, pool, "prov-reset@example.com", false)
	tok, _ := iss.Issue(userID, 1)
	ctx := context.Background()

	oldAt := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
	blob, _ := json.Marshal(resumeextract.Structured{
		FullName: "Ada Lovelace", Email: "ada@example.com", Summary: "stale",
	})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2,
		 resume_structured = $3, resume_structured_uploaded_at = $2 WHERE id = $1`,
		userID, oldAt, blob); err != nil {
		t.Fatalf("seed structure: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET resume_uploaded_at = $2 WHERE id = $1`,
		userID, oldAt.Add(time.Hour)); err != nil {
		t.Fatalf("stale upload: %v", err)
	}
	seedBankedCareer(t, h.queries, userID)

	jobID := seedJobSlug(t, pool, "prov-reset-"+uuid.NewString()[:8])
	base, err := h.cvStore.Create(ctx, userID, "My CV", cv.DefaultTemplateID, cv.Document{
		Header:  cv.Header{FullName: "Base Name"},
		Summary: "base keep me",
		Skills:  []cv.SkillGroup{{Items: []string{"BaseSkill"}}},
	})
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	tailored, err := h.cvStore.CreateTailored(ctx, userID, jobID, "T", cv.DefaultTemplateID, cv.Document{
		Header:  cv.Header{},
		Summary: "tailored keep me",
		Skills:  []cv.SkillGroup{{Items: []string{"TailorSkill"}}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	app := buildResetApp(h, iss)
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/"+tailored.ID.String()+"/reset-from-resume", tok, nil)
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s, want 200", resp.StatusCode, body)
	}
	resp.Body.Close()

	// Seed composition must still strip superseded summary — only identity from the blob.
	st, ok, err := h.seedSource().Structured(ctx, userID)
	if err != nil || !ok {
		t.Fatalf("seed Structured: ok=%v err=%v", ok, err)
	}
	if st.Summary != "" || len(st.Skills) != 0 {
		t.Fatalf("seed leaked superseded semantics: summary=%q skills=%v", st.Summary, st.Skills)
	}

	got, err := h.cvStore.Get(ctx, tailored.ID, userID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Document.Header.FullName != "Ada Lovelace" || got.Document.Header.Email != "ada@example.com" {
		t.Fatalf("header = %+v, want provisional contacts", got.Document.Header)
	}
	if got.Document.Summary != "tailored keep me" {
		t.Fatalf("summary = %q, want prior tailored summary kept", got.Document.Summary)
	}
	if len(got.Document.Skills) == 0 || got.Document.Skills[0].Items[0] != "TailorSkill" {
		t.Fatalf("skills = %+v, want prior tailored skills kept", got.Document.Skills)
	}
	if len(got.Document.Experience) == 0 {
		t.Fatal("want banked experience on reset")
	}

	gotBase, ok, err := h.cvStore.BaseCV(ctx, userID)
	if err != nil || !ok {
		t.Fatalf("BaseCV: ok=%v err=%v", ok, err)
	}
	if gotBase.ID != base.ID {
		t.Fatalf("base id = %s, want %s", gotBase.ID, base.ID)
	}
	if gotBase.Document.Summary != "base keep me" {
		t.Fatalf("base summary = %q, want prior base summary kept", gotBase.Document.Summary)
	}
	if len(gotBase.Document.Skills) == 0 || gotBase.Document.Skills[0].Items[0] != "BaseSkill" {
		t.Fatalf("base skills = %+v, want prior base skills kept", gotBase.Document.Skills)
	}
}

func TestResetCVFromResume_OtherOwner404(t *testing.T) {
	f := newResetFixture(t)
	otherID := seedAccount(t, f.pool, "other-reset@example.com", false)
	otherTok, err := f.iss.Issue(otherID, 1)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	path := "/api/v1/me/cvs/" + f.tailored.ID.String() + "/reset-from-resume"
	resp := doCV(t, f.app, fiber.MethodPost, path, otherTok, nil)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestResetCVFromResume_DoesNotChangeProfile(t *testing.T) {
	f := newResetFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO user_profiles (user_id, specializations, skills, excluded_skills)
		 VALUES ($1, ARRAY['backend'], ARRAY['go'], ARRAY[]::text[])`,
		f.userID); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	path := "/api/v1/me/cvs/" + f.tailored.ID.String() + "/reset-from-resume"
	resp := doCV(t, f.app, fiber.MethodPost, path, f.token, nil)
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	var skills []string
	if err := f.pool.QueryRow(ctx,
		`SELECT skills FROM user_profiles WHERE user_id = $1`, f.userID).Scan(&skills); err != nil {
		t.Fatalf("read profile: %v", err)
	}
	if len(skills) != 1 || skills[0] != "go" {
		t.Fatalf("profile skills = %v, want [go] unchanged (Reset must not merge)", skills)
	}
}

// A bank role with more banked, publishable claims than cv.MaxBullets is exactly the seed
// CommitDocument used to truncate silently (it sanitizes before diffing, so the refuse
// guard never saw the overflow). Reset must refuse instead — and because the tailored
// target now commits before the base refresh, a refusal must leave BOTH untouched rather
// than a base already rewritten under a request that reports failure.
func TestResetCVFromResume_RefusesWhenTheBankSeedExceedsTheBulletCap(t *testing.T) {
	prevMax := cv.MaxBullets
	cv.SetMaxBullets(20)
	t.Cleanup(func() { cv.SetMaxBullets(prevMax) })

	h, iss, pool := newTailorAPI(t)
	bank := experience.NewStore(experience.NewQueriesRepository(h.queries))
	h.seeder = bankedSeeder{resume: h.resume, bank: bank}

	userID := seedAccount(t, pool, "overcap-reset@example.com", false)
	tok, _ := iss.Issue(userID, 1)
	ctx := context.Background()

	emp, err := bank.CreateEmployment(ctx, userID, experience.Employment{
		Kind: experience.KindJob, Company: "Neon", Role: "Staff Engineer",
		Start: &perioddate.PeriodDate{Year: 2018}, End: &perioddate.PeriodDate{Year: 2024},
	})
	if err != nil {
		t.Fatalf("CreateEmployment: %v", err)
	}
	for i := 0; i < cv.MaxBullets+1; i++ {
		if _, err := bank.AddAtom(ctx, userID, experience.Atom{
			EmploymentID: &emp.ID,
			Claim:        fmt.Sprintf("Banked achievement %d", i+1),
			Provenance:   experience.ProvenanceStatedInChat,
		},
			experience.AuthorQuoted,
		); err != nil {
			t.Fatalf("AddAtom %d: %v", i, err)
		}
	}

	store := h.cvStore
	base, err := store.Create(ctx, userID, "My CV", cv.DefaultTemplateID, cv.Document{
		Header: cv.Header{FullName: "Old Base"}, Summary: "old base summary",
	})
	if err != nil {
		t.Fatalf("create base: %v", err)
	}
	jobID := seedJobSlug(t, pool, "overcap-reset-job")
	tailored, err := store.CreateTailored(ctx, userID, jobID, "Tailored for Role", cv.DefaultTemplateID, cv.Document{
		Header: cv.Header{FullName: "Old Tailored"}, Summary: "old tailored summary",
	})
	if err != nil {
		t.Fatalf("create tailored: %v", err)
	}

	app := buildResetApp(h, iss)
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/"+tailored.ID.String()+"/reset-from-resume", tok, nil)
	if resp.StatusCode != fiber.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 409: %s", resp.StatusCode, body)
	}

	gotTailored, err := store.Get(ctx, tailored.ID, userID)
	if err != nil {
		t.Fatalf("get tailored: %v", err)
	}
	if gotTailored.Document.Summary != "old tailored summary" {
		t.Fatalf("tailored summary = %q, want untouched by a refused reset", gotTailored.Document.Summary)
	}
	gotBase, ok, err := store.BaseCV(ctx, userID)
	if err != nil || !ok {
		t.Fatalf("BaseCV: ok=%v err=%v", ok, err)
	}
	if gotBase.ID != base.ID || gotBase.Document.Summary != "old base summary" {
		t.Fatalf("base = %+v, want untouched — the target failed before the base refresh ran", gotBase.Document)
	}
}

// TestResetCVFromResume_AlignsSkillSurfaces: resetting a tailored copy whose seed says IaC
// stores the vacancy's preferred form on the tailored document, while the base keeps IaC.
func TestResetCVFromResume_AlignsSkillSurfaces(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	userID := seedAccount(t, pool, "reset-align@example.com", false)
	tok, err := iss.Issue(userID, 1)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	ctx := context.Background()
	blob, _ := json.Marshal(resumeextract.Structured{
		FullName: "Ada Lovelace",
		Email:    "ada@example.com",
		Summary:  "Platform engineer",
		Skills:   []string{"IaC", "Terraform"},
	})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = now(),
		 resume_structured = $2, resume_structured_uploaded_at = now(),
		 resume_structured_model = 'test' WHERE id = $1`,
		userID, blob); err != nil {
		t.Fatalf("seed structured: %v", err)
	}

	store := h.cvStore
	if _, err := store.Create(ctx, userID, "My CV", cv.DefaultTemplateID, cv.Document{
		Header: cv.Header{FullName: "Old Base"},
		Skills: []cv.SkillGroup{{Items: []string{"Go"}}},
	}); err != nil {
		t.Fatalf("create base: %v", err)
	}
	jobID := seedJobSlugDesc(t, pool, "reset-align-"+uuid.NewString()[:8],
		"We practice infrastructure as code.")
	tailored, err := store.CreateTailored(ctx, userID, jobID, "Tailored", cv.DefaultTemplateID, cv.Document{
		Header: cv.Header{FullName: "Old Tailored"},
		Skills: []cv.SkillGroup{{Items: []string{"Go"}}},
	})
	if err != nil {
		t.Fatalf("create tailored: %v", err)
	}

	app := buildResetApp(h, iss)
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/"+tailored.ID.String()+"/reset-from-resume", tok, nil)
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var out struct {
		Data cvResponse `json:"data"`
	}
	decodeJSON(t, resp, &out)
	if out.Data.Document.Summary != "Platform engineer" {
		t.Fatalf("tailored summary = %q, want current extract summary", out.Data.Document.Summary)
	}
	if len(out.Data.Document.Skills) == 0 || out.Data.Document.Skills[0].Items[0] != "infrastructure as code" {
		t.Fatalf("tailored skills = %+v, want infrastructure as code", out.Data.Document.Skills)
	}

	base, ok, err := store.BaseCV(ctx, userID)
	if err != nil || !ok {
		t.Fatalf("BaseCV: ok=%v err=%v", ok, err)
	}
	if base.Document.Summary != "Platform engineer" {
		t.Fatalf("base summary = %q, want current extract summary", base.Document.Summary)
	}
	if len(base.Document.Skills) == 0 || base.Document.Skills[0].Items[0] != "IaC" {
		t.Fatalf("base skills = %+v, want IaC (résumé form, not JD)", base.Document.Skills)
	}
}

// TestResetCVFromResume_CurrentExtractAppliesSkillsAndSummary: a current stamp with
// summary and skills must land them on reset (seed wins over whatever was on the page).
func TestResetCVFromResume_CurrentExtractAppliesSkillsAndSummary(t *testing.T) {
	h, iss, pool := newTailorAPI(t)
	userID := seedAccount(t, pool, "reset-current@example.com", false)
	tok, _ := iss.Issue(userID, 1)
	ctx := context.Background()
	blob, _ := json.Marshal(resumeextract.Structured{
		FullName: "Ada Lovelace",
		Email:    "ada@example.com",
		Summary:  "Staff platform engineer",
		Skills:   []string{"Go", "Kafka"},
	})
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = now(),
		 resume_structured = $2, resume_structured_uploaded_at = now(),
		 resume_structured_model = 'test' WHERE id = $1`,
		userID, blob); err != nil {
		t.Fatalf("seed structured: %v", err)
	}

	store := h.cvStore
	if _, err := store.Create(ctx, userID, "My CV", cv.DefaultTemplateID, cv.Document{
		Header:  cv.Header{FullName: "Old Base"},
		Summary: "old base summary",
		Skills:  []cv.SkillGroup{{Items: []string{"OldBase"}}},
	}); err != nil {
		t.Fatalf("create base: %v", err)
	}
	jobID := seedJobSlug(t, pool, "reset-current-"+uuid.NewString()[:8])
	tailored, err := store.CreateTailored(ctx, userID, jobID, "Tailored", cv.DefaultTemplateID, cv.Document{
		Header:  cv.Header{FullName: "Old Tailored"},
		Summary: "old tailored summary",
		Skills:  []cv.SkillGroup{{Items: []string{"OldTailor"}}},
	})
	if err != nil {
		t.Fatalf("create tailored: %v", err)
	}

	app := buildResetApp(h, iss)
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/"+tailored.ID.String()+"/reset-from-resume", tok, nil)
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var out struct {
		Data cvResponse `json:"data"`
	}
	decodeJSON(t, resp, &out)
	if out.Data.Document.Summary != "Staff platform engineer" {
		t.Fatalf("tailored summary = %q, want extract", out.Data.Document.Summary)
	}
	if len(out.Data.Document.Skills) == 0 || len(out.Data.Document.Skills[0].Items) != 2 ||
		out.Data.Document.Skills[0].Items[0] != "Go" || out.Data.Document.Skills[0].Items[1] != "Kafka" {
		t.Fatalf("tailored skills = %+v, want Go/Kafka from extract", out.Data.Document.Skills)
	}

	base, ok, err := store.BaseCV(ctx, userID)
	if err != nil || !ok {
		t.Fatalf("BaseCV: ok=%v err=%v", ok, err)
	}
	if base.Document.Summary != "Staff platform engineer" {
		t.Fatalf("base summary = %q, want extract", base.Document.Summary)
	}
	if len(base.Document.Skills) == 0 || base.Document.Skills[0].Items[0] != "Go" {
		t.Fatalf("base skills = %+v, want extract", base.Document.Skills)
	}
}

func TestResetBaseCVFromResume_HappyPath(t *testing.T) {
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
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/base/reset-from-resume", tok, nil)
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

func TestResetBaseCVFromResume_NoSeed409(t *testing.T) {
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
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/base/reset-from-resume", tok, nil)
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

func TestResetBaseCVFromResume_Unauth401(t *testing.T) {
	h, iss, _ := newTailorAPI(t)
	app := buildResetApp(h, iss)
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/base/reset-from-resume", "", nil)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestResetBaseCVFromResume_OtherUserLeavesOwnersBase(t *testing.T) {
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
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/base/reset-from-resume", otherTok, nil)
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
