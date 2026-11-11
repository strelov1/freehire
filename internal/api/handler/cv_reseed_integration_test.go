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
	app.Get("/api/v1/me/cvs/:id", keyAuth, cvContract(h))
	app.Post("/api/v1/me/cvs/base/reseed", saved, cvContract(h))
	app.Post("/api/v1/me/cvs/:id/reseed", saved, cvContract(h))
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

func TestReseedCV_HappyPath(t *testing.T) {
	f := newResetFixture(t)
	path := "/api/v1/me/cvs/" + f.tailored.ID.String() + "/reseed"
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

	// Refreshing a tailored copy never writes a base: hand edits there are the candidate's.
	base, ok, err := f.store.BaseCV(context.Background(), f.userID)
	if err != nil || !ok {
		t.Fatalf("BaseCV: ok=%v err=%v", ok, err)
	}
	if base.ID != f.base.ID || base.Document.Header.FullName != "Old Base" || base.Document.Summary != "old base summary" {
		t.Fatalf("base was touched by a tailored reseed: %+v / %q", base.Document.Header, base.Document.Summary)
	}
}

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

func TestReseedCV_NoSeed409(t *testing.T) {
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
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/"+tailored.ID.String()+"/reseed", tok, nil)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s, want 409", resp.StatusCode, body)
	}
}

// Banked experience without a current structured résumé is not a usable seed — reset must
// refuse rather than blank the tailored header.
func TestReseedCV_BankOnlySeed409(t *testing.T) {
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
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/"+tailored.ID.String()+"/reseed", tok, nil)
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
func TestReseedCV_IdentityOnlySeed409(t *testing.T) {
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
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/"+tailored.ID.String()+"/reseed", tok, nil)
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

func TestReseedCV_ProvisionalContactsPlusBankSucceeds(t *testing.T) {
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
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/"+tailored.ID.String()+"/reseed", tok, nil)
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

func TestReseedCV_OtherOwner404(t *testing.T) {
	f := newResetFixture(t)
	otherID := seedAccount(t, f.pool, "other-reset@example.com", false)
	otherTok, err := f.iss.Issue(otherID, 1)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	path := "/api/v1/me/cvs/" + f.tailored.ID.String() + "/reseed"
	resp := doCV(t, f.app, fiber.MethodPost, path, otherTok, nil)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestReseedCV_DoesNotChangeProfile(t *testing.T) {
	f := newResetFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx,
		`INSERT INTO user_profiles (user_id, specializations, skills, excluded_skills)
		 VALUES ($1, ARRAY['backend'], ARRAY['go'], ARRAY[]::text[])`,
		f.userID); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	path := "/api/v1/me/cvs/" + f.tailored.ID.String() + "/reseed"
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

// A bank employment growing past the bullet cap over years of real use used to dead-end
// this route forever: the seed built from it was already over cv.MaxBullets, and
// CommitDocument's cap guard refused the whole write on every retry, with no user-facing
// recovery. cv.Seed now caps every Bullets list at MaxBullets before a Document is ever
// built (internal/candidate/cv/seed.go), keeping the MOST RECENTLY banked achievements —
// so this reseed must succeed, not refuse, and must refresh the base CV too.
func TestReseedCV_CapsAnOvercapBankBucketInsteadOfRefusing(t *testing.T) {
	prevMax := cv.MaxBullets
	cv.SetMaxBullets(20)
	t.Cleanup(func() { cv.SetMaxBullets(prevMax) })

	h, iss, pool := newTailorAPI(t)
	bank := experience.NewStore(experience.NewQueriesRepository(h.queries))
	h.seeder = bankedSeeder{resume: h.resume, bank: bank}

	userID := seedAccount(t, pool, "overcap-reset@example.com", false)
	tok, _ := iss.Issue(userID, 1)
	ctx := context.Background()

	// haveSource (cv_seed.go's bankedSeeder.Structured) requires the résumé's own
	// structure, independent of the bank — TestReseedCV_BankOnlySeed409 pins that a bank
	// alone is not "usable". Identity fields only; work history below comes from the bank.
	blob, _ := json.Marshal(resumeextract.Structured{FullName: "Neon Candidate"})
	uploadedAt := time.Now().Add(-time.Hour).Truncate(time.Microsecond)
	if _, err := pool.Exec(ctx,
		`UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2,
		 resume_structured = $3, resume_structured_uploaded_at = $2 WHERE id = $1`,
		userID, uploadedAt, blob); err != nil {
		t.Fatalf("seed structure: %v", err)
	}

	emp, err := bank.CreateEmployment(ctx, userID, experience.Employment{
		Kind: experience.KindJob, Company: "Neon", Role: "Staff Engineer",
		Start: &perioddate.PeriodDate{Year: 2018}, End: &perioddate.PeriodDate{Year: 2024},
	})
	if err != nil {
		t.Fatalf("CreateEmployment: %v", err)
	}
	// Oldest first, matching the store's own created_at ordering — achievement 1 is the
	// oldest and must be the one dropped once the bucket is capped.
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
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/"+tailored.ID.String()+"/reseed", tok, nil)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, body)
	}

	gotTailored, err := store.Get(ctx, tailored.ID, userID)
	if err != nil {
		t.Fatalf("get tailored: %v", err)
	}
	if len(gotTailored.Document.Experience) != 1 {
		t.Fatalf("experience = %+v, want the one banked employment", gotTailored.Document.Experience)
	}
	bullets := gotTailored.Document.Experience[0].Bullets
	if len(bullets) != cv.MaxBullets {
		t.Fatalf("bullets = %d, want capped at %d", len(bullets), cv.MaxBullets)
	}
	if bullets[0] != "Banked achievement 2" {
		t.Errorf("first surviving bullet = %q, want the oldest one dropped", bullets[0])
	}
	last := fmt.Sprintf("Banked achievement %d", cv.MaxBullets+1)
	if got := bullets[len(bullets)-1]; got != last {
		t.Errorf("last surviving bullet = %q, want %q — the most recently banked", got, last)
	}

	// A tailored reseed never writes the base.
	gotBase, ok, err := store.BaseCV(ctx, userID)
	if err != nil || !ok {
		t.Fatalf("BaseCV: ok=%v err=%v", ok, err)
	}
	if gotBase.ID != base.ID || gotBase.Document.Summary != "old base summary" || len(gotBase.Document.Experience) != 0 {
		t.Fatalf("base was touched: %+v", gotBase.Document)
	}
}

// TestReseedCV_AlignsSkillSurfaces: resetting a tailored copy whose seed says IaC
// stores the vacancy's preferred form on the tailored document, while the base keeps IaC.
func TestReseedCV_AlignsSkillSurfaces(t *testing.T) {
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
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/"+tailored.ID.String()+"/reseed", tok, nil)
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
	if base.Document.Summary != "" || len(base.Document.Skills) != 1 || base.Document.Skills[0].Items[0] != "Go" {
		t.Fatalf("base = %q / %+v, want it left alone by a tailored reseed", base.Document.Summary, base.Document.Skills)
	}
}

// TestReseedCV_CurrentExtractAppliesSkillsAndSummary: a current stamp with
// summary and skills must land them on reset (seed wins over whatever was on the page).
func TestReseedCV_CurrentExtractAppliesSkillsAndSummary(t *testing.T) {
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
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/"+tailored.ID.String()+"/reseed", tok, nil)
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
	if base.Document.Summary != "old base summary" || base.Document.Skills[0].Items[0] != "OldBase" {
		t.Fatalf("base = %q / %+v, want it left alone by a tailored reseed", base.Document.Summary, base.Document.Skills)
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

func TestReseedBaseCV_Unauth401(t *testing.T) {
	h, iss, _ := newTailorAPI(t)
	app := buildResetApp(h, iss)
	resp := doCV(t, app, fiber.MethodPost, "/api/v1/me/cvs/base/reseed", "", nil)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
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
