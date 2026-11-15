package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unsafe"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/platform/tokencrypt"
)

// TestContractAuthMatchesRouter holds the contract's @useAuth to the gate the router really
// mounts. The contract only documents who may call an operation (ogen never sees it); the
// Fiber middleware decides. Without this test a route could say "sign-in required" in the
// docs while the router leaves it open, or the other way round.
//
// Every route is mounted through the real mountRoutes with zero-valued handlers and
// recording gates; the contract handler is a stub that answers with the gates it saw.
func TestContractAuthMatchesRouter(t *testing.T) {
	raw, err := os.ReadFile("../../../openapi/api.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Security []map[string][]string                 `json:"security"`
		Paths    map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}

	app := mountRecordingRoutes(t)
	param := regexp.MustCompile(`\{[^}]+\}`)
	checked := 0
	for path, ops := range spec.Paths {
		for method, rawOp := range ops {
			var op struct {
				OperationID string                 `json:"operationId"`
				Security    *[]map[string][]string `json:"security"`
			}
			if json.Unmarshal(rawOp, &op) != nil || op.OperationID == "" {
				continue // "parameters" and other non-operation keys
			}
			security := spec.Security
			if op.Security != nil {
				security = *op.Security
			}

			url := "/api/v1" + param.ReplaceAllString(path, "1")
			req := httptest.NewRequestWithContext(context.Background(), strings.ToUpper(method), url, nil)
			resp, err := app.Test(req, -1)
			if err != nil {
				t.Fatalf("%s %s: %v", method, url, err)
			}
			_ = resp.Body.Close()
			gates := resp.Header.Get("X-Test-Gates")
			if resp.StatusCode != fiber.StatusTeapot {
				t.Errorf("%s (%s %s): did not reach the contract handler (status %d, gates %q)",
					op.OperationID, method, path, resp.StatusCode, gates)
				continue
			}
			if !gatesMatch(security, gates) {
				t.Errorf("%s (%s %s): contract security %v, router mounts gates %q",
					op.OperationID, method, path, security, gates)
			}
			checked++
		}
	}
	if checked < 300 {
		t.Fatalf("checked only %d operations; the walk is not covering the contract", checked)
	}
}

// authGates are the gates that authenticate a caller, by what they accept.
var authGates = map[string]string{
	"cookie":        "cookie",
	"key":           "key",
	"cvKey":         "key",
	"autoApplyGate": "key",
}

// callerGates need a caller already in hand, so a public operation must mount none of them.
var callerGates = []string{"paidAI", "moderator", "admin"}

// gatesMatch reports whether the comma-separated gates a route mounts implement security:
// a public operation (an empty requirement) mounts no gate that needs a caller; an
// authenticated one mounts at least one authenticating gate, and every one of them accepts
// what the contract says (a cookie, or a cookie or a key).
func gatesMatch(security []map[string][]string, gates string) bool {
	var open, key bool
	for _, req := range security {
		switch {
		case len(req) == 0:
			open = true
		case req["PersonalApiKey"] != nil:
			key = true
		}
	}
	var accepts []string
	for g := range strings.SplitSeq(gates, ",") {
		if kind, ok := authGates[g]; ok {
			accepts = append(accepts, kind)
		} else if open && slices.Contains(callerGates, g) {
			return false
		}
	}
	if open {
		return len(accepts) == 0
	}
	want := "cookie"
	if key {
		want = "key"
	}
	return len(accepts) > 0 && !slices.ContainsFunc(accepts, func(k string) bool { return k != want })
}

func TestGatesMatch(t *testing.T) {
	public := []map[string][]string{{}}
	cookie := []map[string][]string{{"CookieSession": {}}}
	either := []map[string][]string{{"CookieSession": {}}, {"PersonalApiKey": {}}}
	cases := []struct {
		name     string
		security []map[string][]string
		gates    string
		want     bool
	}{
		{"public, no gate", public, "", true},
		{"public, optional", public, "optional", true},
		{"public, optional then cookie", public, "optional,cookie", false},
		{"public, paidAI", public, "paidAI", false},
		{"cookie, cookie", cookie, "cookie", true},
		{"cookie, cookie then moderator", cookie, "cookie,moderator", true},
		{"cookie, open", cookie, "optional", false},
		{"cookie, key", cookie, "key", false},
		{"cookie, cookie then key", cookie, "cookie,key", false},
		{"either, key", either, "key", true},
		{"either, cvKey then paidAI", either, "cvKey,paidAI", true},
		{"either, cookie only", either, "cookie", false},
	}
	for _, c := range cases {
		if got := gatesMatch(c.security, c.gates); got != c.want {
			t.Errorf("%s: gatesMatch(%q) = %v, want %v", c.name, c.gates, got, c.want)
		}
	}
}

// mountRecordingRoutes mounts every route with gates that record their name in a response
// header, and a contract stub that answers 418 so a request that reached it is unmistakable.
func mountRecordingRoutes(t *testing.T) *fiber.App {
	t.Helper()
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Use(recover.New())
	gate := func(name string) fiber.Handler {
		return func(c *fiber.Ctx) error {
			seen := name
			if prev := string(c.Response().Header.Peek("X-Test-Gates")); prev != "" {
				seen = prev + "," + name
			}
			c.Set("X-Test-Gates", seen)
			return c.Next()
		}
	}
	mw := middleware{
		optional:       gate("optional"),
		key:            gate("key"),
		cvKey:          gate("cvKey"),
		autoApplyGate:  gate("autoApplyGate"),
		cookie:         gate("cookie"),
		optionalCookie: gate("optionalCookie"),
		moderator:      gate("moderator"),
		paidAI:         func(plan.Feature) fiber.Handler { return gate("paidAI") },
	}
	contract := func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusTeapot) }

	var h routeHandlers
	allocate(reflect.ValueOf(&h).Elem())
	// Both billing providers mount their routes only when configured.
	h.billingHandlers = newBillingHandlers(
		billing.New(billing.Config{APIKey: "k", WebhookSecret: "s", Prices: []string{"price"}}, nil),
		billing.NewRevenueCat(billing.RevenueCatConfig{APIKey: "k", WebhookSecret: "s", Entitlement: "pro"}, nil), nil)
	// The Gmail and hosted-mailbox routes mount only when configured; the zero values
	// stand in for a configuration, since no request ever reaches them.
	h.gmailConnector = &gmailsync.Connector{}
	h.gmailCipher = &tokencrypt.Cipher{}
	h.mailDomain = "mail.example"
	h.adminOnly = gate("admin")

	api := app.Group("/api/v1")
	// Mounted by Register itself, just before mountRoutes.
	h.emailPrefsHandlers.register(api, mw, contract)
	(&API{}).mountRoutes(app, api, mw, contract, h)
	return app
}

// allocate gives every nil handler pointer in v (embedded structs included) a zero value,
// so each register runs as it does in production, minus its dependencies.
func allocate(v reflect.Value) {
	for i := range v.NumField() {
		// The handler fields are unexported, so reflect hands out read-only values; NewAt
		// re-derives a settable one at the same address.
		f := reflect.NewAt(v.Field(i).Type(), unsafe.Pointer(v.Field(i).UnsafeAddr())).Elem()
		switch {
		case f.Kind() == reflect.Pointer && f.IsNil():
			f.Set(reflect.New(f.Type().Elem()))
		case f.Kind() == reflect.Struct:
			allocate(f)
		}
	}
}
