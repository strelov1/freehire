package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/cvedit"
)

// signedIn stands in for the cookie middleware: it puts a caller in the context the way
// RequireAuth would, so a handler test can exercise the route without a session.
func signedIn(userID int64) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Locals(auth.LocalsUserID, userID)
		return c.Next()
	}
}

// TestCVRegister_TracerLinksToggleIsCookieOnly pins the gate on the consent decision against the
// real register(). Cookie-only is the enforcement, not a preference: the tailoring agent
// authenticates with a CLI credential, and consent to track a third party is the candidate's to
// give. Widening this to `key` or `cvKey` would let an agent grant it on their behalf, and this
// test is the tripwire.
func TestCVRegister_TracerLinksToggleIsCookieOnly(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api/v1")
	(&cvHandlers{}).register(api, middleware{
		key:    namedGate("key"),
		cvKey:  namedGate("cvKey"),
		cookie: namedGate("cookie"),
	})

	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/me/cvs/"+uuid.New().String()+"/tracer-links", strings.NewReader(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if got := string(body); got != "cookie" {
		t.Errorf("PUT /me/cvs/:id/tracer-links is gated by %q, want %q", got, "cookie")
	}
}

// TestTracerLinksToggleIsNotAnEditablePath proves the toggle cannot be reached through the edit
// path at all. cvedit validates paths by reflection over State, so this is not a list that can
// drift out of step: if the flag were ever added to State it would become addressable, and the
// tailoring agent's batch could set it.
//
// Keeping it out of State also keeps it out of the revision history, which matters more than it
// looks: inside State the flag would be snapshotted into every revision, and undoing an unrelated
// edit to a bullet point would silently re-grant or revoke consent to track somebody.
func TestTracerLinksToggleIsNotAnEditablePath(t *testing.T) {
	if _, err := cvedit.ParsePath("tracer_links_enabled"); err == nil {
		t.Error("cvedit accepts the path \"tracer_links_enabled\" — the toggle is editable, " +
			"so an agent can set it and an undo can revoke it")
	}
	for _, p := range cvedit.Paths() {
		if strings.Contains(string(p), "tracer") {
			t.Errorf("cvedit advertises the editable path %q", p)
		}
	}
}

// tracerToggleAPI wires the toggle route over a CV owned by the caller, with NO visitor salt
// configured — the state every deployment is in until one is set.
func tracerToggleAPI(t *testing.T) (*fiber.App, uuid.UUID) {
	t.Helper()
	const owner = int64(7)
	id := uuid.MustParse("77777777-7777-4777-8777-777777777777")
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	api := app.Group("/api/v1")
	h := &cvHandlers{cvStore: cv.NewStore(&cvRepo{id: id, userID: owner, data: []byte(`{}`)})}
	h.register(api, middleware{key: signedIn(owner), cvKey: signedIn(owner), cookie: signedIn(owner)})
	return app, id
}

// TestEnablingTracingWithoutASaltIsRefused: the salt is what stops the stored visitor hash being
// reversed by walking the address space. Without one there is no honest way to count visitors, so
// the answer is to refuse the consent rather than to accept it and quietly record less.
//
// The store here would accept the write, so a 409 can only come from the check in front of it.
func TestEnablingTracingWithoutASaltIsRefused(t *testing.T) {
	app, id := tracerToggleAPI(t)

	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/me/cvs/"+id.String()+"/tracer-links", strings.NewReader(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != fiber.StatusConflict {
		t.Errorf("enabling without a salt = %d, want %d", resp.StatusCode, fiber.StatusConflict)
	}
}

// Turning tracing OFF must never be blocked by configuration. A candidate withdrawing consent is
// not asking the deployment for a favour, and a missing salt is our problem, not theirs.
func TestDisablingTracingIsAllowedWithoutASalt(t *testing.T) {
	app, id := tracerToggleAPI(t)

	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/me/cvs/"+id.String()+"/tracer-links", strings.NewReader(`{"enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode == fiber.StatusConflict {
		t.Error("withdrawing consent was refused for want of a salt")
	}
}
