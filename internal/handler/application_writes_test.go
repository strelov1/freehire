package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/jobtracking"
)

// notFoundRepo answers every application-addressed write the way the database does
// when the id names nothing the caller holds — which is the same answer for a row
// that never existed and a row belonging to somebody else, because each statement is
// scoped by user_id and neither matches.
type notFoundRepo struct{ stubTrackingRepo }

func (notFoundRepo) TrackApplication(context.Context, int64, int64, *string, *string, string) (jobtracking.Interaction, error) {
	return jobtracking.Interaction{}, jobtracking.ErrApplicationNotFound
}

func (notFoundRepo) ClearApplicationProgress(context.Context, int64, int64) (jobtracking.Interaction, error) {
	return jobtracking.Interaction{}, jobtracking.ErrApplicationNotFound
}

func (notFoundRepo) UntrackApplication(context.Context, int64, int64) (jobtracking.Interaction, error) {
	return jobtracking.Interaction{}, jobtracking.ErrApplicationNotFound
}

// appWriteApp mounts the three application-addressed routes on the given repository.
func appWriteApp(t *testing.T, repo jobtracking.Repository) (*fiber.App, *auth.Issuer) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &trackingHandlers{tracking: jobtracking.New(repo)}
	app := fiber.New()
	app.Patch("/me/applications/:id", auth.RequireAuth(iss, testVersions), h.TrackApplication)
	app.Delete("/me/applications/:id", auth.RequireAuth(iss, testVersions), h.UntrackApplication)
	app.Delete("/me/applications/:id/stage", auth.RequireAuth(iss, testVersions), h.ClearApplicationStage)
	return app, iss
}

func appWriteReq(t *testing.T, app *fiber.App, iss *auth.Issuer, method, path, body string) *http.Response {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequestWithContext(context.Background(), method, path, nil)
	} else {
		r = httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	tok, err := iss.Issue(7, testTokenVersion)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: tok})
	res, err := app.Test(r, -1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return res
}

func TestApplicationWritesRejectABadBodyBeforeTheLookup(t *testing.T) {
	app, iss := appWriteApp(t, stubTrackingRepo{})
	for _, tc := range []struct{ name, body string }{
		{"empty track", `{}`},
		{"unknown stage", `{"stage":"banana"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := appWriteReq(t, app, iss, fiber.MethodPatch, "/me/applications/a42", tc.body)
			defer res.Body.Close()
			if res.StatusCode != fiber.StatusBadRequest {
				t.Errorf("status = %d, want 400 — the body is wrong whichever way the row was named", res.StatusCode)
			}
		})
	}
}

func TestApplicationWritesAcceptAValidStage(t *testing.T) {
	app, iss := appWriteApp(t, stubTrackingRepo{})
	res := appWriteReq(t, app, iss, fiber.MethodPatch, "/me/applications/a42", `{"stage":"interview"}`)
	defer res.Body.Close()
	if res.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
}

// The whole point of the routes: the id form that names an application directly is the
// one an application whose posting was pruned carries, and it must be writable.
func TestApplicationWritesAcceptBothIDForms(t *testing.T) {
	app, iss := appWriteApp(t, stubTrackingRepo{})
	for _, id := range []string{"a42", "senior-go-engineer-stripe-mfzg42lt"} {
		res := appWriteReq(t, app, iss, fiber.MethodPatch, "/me/applications/"+id, `{"stage":"offer"}`)
		defer res.Body.Close()
		if res.StatusCode != fiber.StatusOK {
			t.Errorf("id %q: status = %d, want 200", id, res.StatusCode)
		}
	}
}

// "Not an id" and "not yours" must be one answer, so neither tells a caller which
// applications exist.
func TestApplicationWritesAnswerOneNotFound(t *testing.T) {
	app, iss := appWriteApp(t, notFoundRepo{})
	for _, tc := range []struct{ name, method, path, body string }{
		{"track", fiber.MethodPatch, "/me/applications/a999", `{"stage":"offer"}`},
		{"untrack", fiber.MethodDelete, "/me/applications/a999", ""},
		{"clear stage", fiber.MethodDelete, "/me/applications/a999/stage", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := appWriteReq(t, app, iss, tc.method, tc.path, tc.body)
			defer res.Body.Close()
			if res.StatusCode != fiber.StatusNotFound {
				t.Errorf("status = %d, want 404", res.StatusCode)
			}
		})
	}
}

func TestApplicationWritesRequireAuthentication(t *testing.T) {
	app, _ := appWriteApp(t, stubTrackingRepo{})
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPatch, "/me/applications/a42", strings.NewReader(`{"stage":"offer"}`))
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
}
