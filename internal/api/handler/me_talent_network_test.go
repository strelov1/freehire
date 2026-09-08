package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// errUniqueViolationStub is what the fake store returns to stand in for another account
// already holding the handle we just minted. It has to be a real *pgconn.PgError:
// pgerr.IsUniqueViolation unwraps to that type, so a plain errors.New would take the
// retry path out of the test without the test noticing.
var errUniqueViolationStub error = &pgconn.PgError{Code: "23505", ConstraintName: "users_talent_handle_key"}

// fakeTalentNetworkStore is a talentNetworkStore backed by an in-memory row, enough to
// exercise the handlers' request parsing and validation without a database.
type fakeTalentNetworkStore struct {
	visibility string
	// handle is the stored talent_handle; empty means the account has never joined.
	handle string
	// structured is the raw resume_structured JSON the mint reads a job title out of.
	structured []byte
	getErr     error
	setErr     error
	setCalls   int
	claimCalls int
	// claimConflicts is how many claim attempts fail with a unique violation before one
	// succeeds, standing in for another account holding the handle we just minted.
	claimConflicts int
}

func (f *fakeTalentNetworkStore) GetTalentNetworkVisibility(context.Context, int64) (db.GetTalentNetworkVisibilityRow, error) {
	if f.getErr != nil {
		return db.GetTalentNetworkVisibilityRow{}, f.getErr
	}
	return db.GetTalentNetworkVisibilityRow{
		TalentNetworkVisibility: f.visibility,
		TalentHandle:            pgtype.Text{String: f.handle, Valid: f.handle != ""},
	}, nil
}

func (f *fakeTalentNetworkStore) GetUserResumeStructuredOnly(context.Context, int64) ([]byte, error) {
	return f.structured, nil
}

func (f *fakeTalentNetworkStore) SetTalentHandleIfUnset(_ context.Context, arg db.SetTalentHandleIfUnsetParams) (int64, error) {
	f.claimCalls++
	if f.claimConflicts > 0 {
		f.claimConflicts--
		return 0, errUniqueViolationStub
	}
	if f.handle != "" {
		return 0, nil
	}
	f.handle = arg.TalentHandle.String
	return 1, nil
}

func (f *fakeTalentNetworkStore) SetTalentNetworkVisibility(_ context.Context, arg db.SetTalentNetworkVisibilityParams) error {
	f.setCalls++
	if f.setErr != nil {
		return f.setErr
	}
	f.visibility = arg.TalentNetworkVisibility
	return nil
}

// talentNetworkApp mounts the talent-network endpoints behind RequireAuth on a handler
// backed by the given in-memory fake store.
func talentNetworkApp(t *testing.T, store *fakeTalentNetworkStore) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := newTalentNetworkHandlers(store)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	g := auth.RequireAuth(iss, testVersions)
	app.Get("/me/talent-network", g, h.GetVisibility)
	app.Put("/me/talent-network", g, h.PutVisibility)
	return app, token
}

func doTalentNetwork(t *testing.T, app *fiber.App, method, body, token string) *http.Response {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequestWithContext(context.Background(), method, "/me/talent-network", nil)
	} else {
		r = httptest.NewRequestWithContext(context.Background(), method, "/me/talent-network", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(r)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	return resp
}

func TestGetTalentNetwork_DefaultsToOff(t *testing.T) {
	store := &fakeTalentNetworkStore{visibility: "off"}
	app, token := talentNetworkApp(t, store)
	resp := doTalentNetwork(t, app, fiber.MethodGet, "", token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got struct {
		Data talentNetworkResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Data.Visibility != "off" {
		t.Errorf("visibility = %q, want off", got.Data.Visibility)
	}
	// A non-member has no handle at all: it is minted on the first join, so an empty
	// one here means "not yet", never "waiting for one".
	if got.Data.Handle != "" {
		t.Errorf("handle = %q, want empty for an account that never joined", got.Data.Handle)
	}
}

// "public" was the third visibility mode until migration 0146 retired it. It gets its own
// test rather than a row in RejectsInvalidValue's table because it is the one invalid
// value that a stale client — an old tab, a cached bundle — will actually send, and
// because the CHECK constraint would reject it anyway: the handler's job is to turn that
// into a 400 rather than a 500 from the database.
func TestPutTalentNetwork_RejectsRetiredPublicValue(t *testing.T) {
	store := &fakeTalentNetworkStore{visibility: "off"}
	app, token := talentNetworkApp(t, store)

	resp := doTalentNetwork(t, app, fiber.MethodPut, `{"visibility":"public"}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("PUT status = %d, want 400", resp.StatusCode)
	}
	if store.setCalls != 0 {
		t.Error("SetTalentNetworkVisibility should not be called for the retired value")
	}
	if store.visibility != "off" {
		t.Errorf("stored visibility = %q, want it untouched at off", store.visibility)
	}
}

func TestPutTalentNetwork_AnonymousRoundTrips(t *testing.T) {
	store := &fakeTalentNetworkStore{visibility: "off"}
	app, token := talentNetworkApp(t, store)

	putResp := doTalentNetwork(t, app, fiber.MethodPut, `{"visibility":"anonymous"}`, token)
	defer putResp.Body.Close()
	if putResp.StatusCode != fiber.StatusOK {
		t.Fatalf("PUT status = %d, want 200", putResp.StatusCode)
	}

	getResp := doTalentNetwork(t, app, fiber.MethodGet, "", token)
	defer getResp.Body.Close()
	var getGot struct {
		Data talentNetworkResponse `json:"data"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&getGot); err != nil {
		t.Fatalf("decode GET: %v", err)
	}
	if getGot.Data.Visibility != "anonymous" {
		t.Errorf("GET visibility after PUT = %q, want anonymous", getGot.Data.Visibility)
	}
}

func TestPutTalentNetwork_RejectsInvalidValue(t *testing.T) {
	cases := []string{`{"visibility":"invalid"}`, `{"visibility":""}`, `{}`}
	for _, body := range cases {
		t.Run(body, func(t *testing.T) {
			store := &fakeTalentNetworkStore{visibility: "off"}
			app, token := talentNetworkApp(t, store)
			resp := doTalentNetwork(t, app, fiber.MethodPut, body, token)
			defer resp.Body.Close()
			if resp.StatusCode != fiber.StatusBadRequest {
				t.Errorf("status = %d, want 400", resp.StatusCode)
			}
			if store.setCalls != 0 {
				t.Error("SetTalentNetworkVisibility should not be called on invalid input")
			}
		})
	}
}

// A CV whose most recent role is a backend one, so the minted handle's readable part is
// predictable. The employer is present precisely so the tests can assert it never
// reaches the handle.
const backendResumeJSON = `{
  "full_name": "Ivan Strelov",
  "experience": [
    {"title": "QA Engineer", "company": "Contoso", "start": {"year": 2020}, "end": {"year": 2023}},
    {"title": "Senior Backend Engineer", "company": "Umbrella", "start": {"year": 2024}, "current": true}
  ]
}`

func TestPutTalentNetwork_MintsAHandleOnFirstJoin(t *testing.T) {
	store := &fakeTalentNetworkStore{visibility: "off", structured: []byte(backendResumeJSON)}
	app, token := talentNetworkApp(t, store)

	got := putTalentNetworkOK(t, app, token, "anonymous")
	if got.Handle == "" {
		t.Fatal("no handle minted on first join — the catalogue would have nothing to link to")
	}
	if !strings.HasPrefix(got.Handle, "backend-") {
		t.Errorf("handle = %q, want it built from the current role's category", got.Handle)
	}
	if store.handle != got.Handle {
		t.Errorf("stored handle = %q, echoed %q — the response must report what was stored", store.handle, got.Handle)
	}
}

// The handle is the URL somebody has already shared. Leaving the network must not burn
// it, and rejoining must not mint a second one.
func TestPutTalentNetwork_RejoiningKeepsTheHandle(t *testing.T) {
	store := &fakeTalentNetworkStore{visibility: "anonymous", handle: "backend-7f2a"}
	app, token := talentNetworkApp(t, store)

	if got := putTalentNetworkOK(t, app, token, "off"); got.Handle != "backend-7f2a" {
		t.Errorf("handle after leaving = %q, want it kept", got.Handle)
	}
	if got := putTalentNetworkOK(t, app, token, "anonymous"); got.Handle != "backend-7f2a" {
		t.Errorf("handle after rejoining = %q, want the original", got.Handle)
	}
	if store.claimCalls != 0 {
		t.Errorf("claimed a handle %d times for an account that already had one", store.claimCalls)
	}
}

// The handle is frozen at mint, and a change of discipline is the loudest reason it
// might drift: the base comes from the current role's category. It must not — the URL
// somebody shared has to keep working after they change jobs.
func TestPutTalentNetwork_ChangingJobsKeepsTheHandle(t *testing.T) {
	store := &fakeTalentNetworkStore{visibility: "off", structured: []byte(backendResumeJSON)}
	app, token := talentNetworkApp(t, store)

	minted := putTalentNetworkOK(t, app, token, "anonymous").Handle
	if !strings.HasPrefix(minted, "backend-") {
		t.Fatalf("handle = %q, want it minted from the backend role", minted)
	}

	// A new CV whose current role is a different discipline entirely.
	store.structured = []byte(`{"experience":[{"title":"Lead Data Engineer","current":true}]}`)
	if got := putTalentNetworkOK(t, app, token, "off").Handle; got != minted {
		t.Errorf("handle after leaving = %q, want %q", got, minted)
	}
	if got := putTalentNetworkOK(t, app, token, "anonymous").Handle; got != minted {
		t.Errorf("handle after rejoining with a new discipline = %q, want the original %q", got, minted)
	}
}

// The account username is derived from the email's local part, so it is usually the
// person's name. It must never end up in the URL of the page that withholds it.
func TestPutTalentNetwork_HandleNeverCarriesTheName(t *testing.T) {
	store := &fakeTalentNetworkStore{visibility: "off", structured: []byte(backendResumeJSON)}
	app, token := talentNetworkApp(t, store)

	got := putTalentNetworkOK(t, app, token, "anonymous")
	for _, forbidden := range []string{"ivan", "strelov", "umbrella", "contoso"} {
		if strings.Contains(strings.ToLower(got.Handle), forbidden) {
			t.Errorf("handle %q contains %q", got.Handle, forbidden)
		}
	}
}

func TestPutTalentNetwork_RetriesAHandleCollision(t *testing.T) {
	store := &fakeTalentNetworkStore{
		visibility:     "off",
		structured:     []byte(backendResumeJSON),
		claimConflicts: 2,
	}
	app, token := talentNetworkApp(t, store)

	got := putTalentNetworkOK(t, app, token, "anonymous")
	if got.Handle == "" {
		t.Fatal("a collision left the account without a handle")
	}
	if store.claimCalls != 3 {
		t.Errorf("claim attempts = %d, want 3 (two collisions then a success)", store.claimCalls)
	}
}

// A candidate can join before uploading anything. Refusing the join because there is no
// title to read would gate membership on the CV pipeline.
func TestPutTalentNetwork_MintsOverAnEmptyCV(t *testing.T) {
	store := &fakeTalentNetworkStore{visibility: "off"}
	app, token := talentNetworkApp(t, store)

	got := putTalentNetworkOK(t, app, token, "anonymous")
	if !strings.HasPrefix(got.Handle, "candidate-") {
		t.Errorf("handle = %q, want the neutral base", got.Handle)
	}
}

// Turning the toggle OFF must not mint anything for an account that never joined —
// otherwise every stray request would burn a handle.
func TestPutTalentNetwork_LeavingDoesNotMint(t *testing.T) {
	store := &fakeTalentNetworkStore{visibility: "off", structured: []byte(backendResumeJSON)}
	app, token := talentNetworkApp(t, store)

	got := putTalentNetworkOK(t, app, token, "off")
	if got.Handle != "" || store.claimCalls != 0 {
		t.Errorf("handle = %q after %d claims, want none of either", got.Handle, store.claimCalls)
	}
}

// putTalentNetworkOK PUTs the given visibility and decodes a 200 response.
func putTalentNetworkOK(t *testing.T, app *fiber.App, token, visibility string) talentNetworkResponse {
	t.Helper()
	resp := doTalentNetwork(t, app, fiber.MethodPut, `{"visibility":"`+visibility+`"}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("PUT %s status = %d, want 200", visibility, resp.StatusCode)
	}
	var got struct {
		Data talentNetworkResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode PUT %s: %v", visibility, err)
	}
	return got.Data
}

func TestTalentNetwork_RequiresAuth(t *testing.T) {
	store := &fakeTalentNetworkStore{visibility: "off"}
	app, _ := talentNetworkApp(t, store)

	getResp := doTalentNetwork(t, app, fiber.MethodGet, "", "")
	defer getResp.Body.Close()
	if getResp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("GET status = %d, want 401", getResp.StatusCode)
	}

	putResp := doTalentNetwork(t, app, fiber.MethodPut, `{"visibility":"anonymous"}`, "")
	defer putResp.Body.Close()
	if putResp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("PUT status = %d, want 401", putResp.StatusCode)
	}
}
