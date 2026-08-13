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
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
)

// fakeTalentNetworkStore is a talentNetworkStore backed by an in-memory row, enough to
// exercise the handlers' request parsing and validation without a database.
type fakeTalentNetworkStore struct {
	visibility string
	publicID   uuid.UUID
	getErr     error
	setErr     error
	setCalls   int
}

func (f *fakeTalentNetworkStore) GetTalentNetworkVisibility(context.Context, int64) (db.GetTalentNetworkVisibilityRow, error) {
	if f.getErr != nil {
		return db.GetTalentNetworkVisibilityRow{}, f.getErr
	}
	return db.GetTalentNetworkVisibilityRow{TalentNetworkVisibility: f.visibility, TalentNetworkPublicID: f.publicID}, nil
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
	id := uuid.New()
	store := &fakeTalentNetworkStore{visibility: "off", publicID: id}
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
	if got.Data.PublicID != id.String() {
		t.Errorf("public_id = %q, want %q", got.Data.PublicID, id.String())
	}
}

func TestPutTalentNetwork_PublicRoundTrips(t *testing.T) {
	store := &fakeTalentNetworkStore{visibility: "off", publicID: uuid.New()}
	app, token := talentNetworkApp(t, store)

	putResp := doTalentNetwork(t, app, fiber.MethodPut, `{"visibility":"public"}`, token)
	defer putResp.Body.Close()
	if putResp.StatusCode != fiber.StatusOK {
		t.Fatalf("PUT status = %d, want 200", putResp.StatusCode)
	}
	var putGot struct {
		Data talentNetworkResponse `json:"data"`
	}
	if err := json.NewDecoder(putResp.Body).Decode(&putGot); err != nil {
		t.Fatalf("decode PUT: %v", err)
	}
	if putGot.Data.Visibility != "public" {
		t.Errorf("PUT visibility = %q, want public", putGot.Data.Visibility)
	}

	getResp := doTalentNetwork(t, app, fiber.MethodGet, "", token)
	defer getResp.Body.Close()
	var getGot struct {
		Data talentNetworkResponse `json:"data"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&getGot); err != nil {
		t.Fatalf("decode GET: %v", err)
	}
	if getGot.Data.Visibility != "public" {
		t.Errorf("GET visibility after PUT = %q, want public", getGot.Data.Visibility)
	}
}

func TestPutTalentNetwork_AnonymousRoundTrips(t *testing.T) {
	store := &fakeTalentNetworkStore{visibility: "off", publicID: uuid.New()}
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
			store := &fakeTalentNetworkStore{visibility: "off", publicID: uuid.New()}
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

func TestTalentNetwork_RequiresAuth(t *testing.T) {
	store := &fakeTalentNetworkStore{visibility: "off", publicID: uuid.New()}
	app, _ := talentNetworkApp(t, store)

	getResp := doTalentNetwork(t, app, fiber.MethodGet, "", "")
	defer getResp.Body.Close()
	if getResp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("GET status = %d, want 401", getResp.StatusCode)
	}

	putResp := doTalentNetwork(t, app, fiber.MethodPut, `{"visibility":"public"}`, "")
	defer putResp.Body.Close()
	if putResp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("PUT status = %d, want 401", putResp.StatusCode)
	}
}
