package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// TestCVRegister_TailoringEntryPointsTakeAKey pins the two routes a CLI needs to ENTER the
// tailoring cycle against the real register().
//
// Every step of tailoring already accepted a key — read the CV, patch it, read the context,
// render the PDF — but the two that hand out the CV id did not, so an agent holding the
// user's own key could drive the cycle and not start it: the id had to be copied out of a
// browser URL first. Listing is a read of the caller's own rows, and the bootstrap creates a
// copy of their own CV and spends their own credits, which a full-scope key can already do
// through the fit analysis and the assistant. Neither is a new capability class; the gap was
// only that the entry points were missed.
//
// What must NOT follow this widening is the scoring and undo surface (ats-delta, job-match,
// revisions). Those stay cookie-only on purpose — see TestCVRegister_ATSDeltaIsCookieOnly.
func TestCVRegister_TailoringEntryPointsTakeAKey(t *testing.T) {
	for _, tc := range []struct {
		name   string
		method string
		path   string
	}{
		{"list tailored CVs", http.MethodGet, "/api/v1/me/cvs"},
		{"tailoring bootstrap", http.MethodPost, "/api/v1/me/cvs/tailor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()
			(&cvHandlers{}).register(app.Group("/api/v1"), middleware{
				key:    namedGate("key"),
				cvKey:  namedGate("cvKey"),
				cookie: namedGate("cookie"),
			})

			resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, nil))
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if got := string(body); got != "key" {
				t.Errorf("%s %s is gated by %q, want %q", tc.method, tc.path, got, "key")
			}
		})
	}
}

// TestCVRegister_AuthoringStaysCookieOnly is the other half of the widening above: creating a
// blank CV, replacing a whole document, and deleting one are the browser's, not a script's.
// A key reaching these would let a leaked credential rewrite or destroy the candidate's CV
// wholesale, which is a different thing from the field-level, evidence-gated PATCH it holds.
func TestCVRegister_AuthoringStaysCookieOnly(t *testing.T) {
	app := fiber.New()
	(&cvHandlers{}).register(app.Group("/api/v1"), middleware{
		key:    namedGate("key"),
		cvKey:  namedGate("cvKey"),
		cookie: namedGate("cookie"),
	})

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/me/cvs", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if got := string(body); got != "cookie" {
		t.Errorf("POST /me/cvs is gated by %q, want %q", got, "cookie")
	}
}
