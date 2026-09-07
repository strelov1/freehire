package handler

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/strelov1/freehire/internal/api/ratelimit"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// TestPublicReadBudgets_ClearTheMeasuredLivePeaks guards the numbers themselves.
// A ceiling below what production already carries would not bound abuse; it would
// cut off a caller this change was designed to tolerate, and do it silently,
// since nothing else in the suite knows what live traffic looks like. The peaks
// are per-IP per-minute over a 4.6-hour window — see the change's design.md.
func TestPublicReadBudgets_ClearTheMeasuredLivePeaks(t *testing.T) {
	const (
		measuredAgentPeak = 184 // one third-party client, held steadily
		measuredReadPeak  = 258
	)
	if agentSearchPerMinute <= measuredAgentPeak {
		t.Errorf("agentSearchPerMinute = %d, must exceed the measured live peak of %d",
			agentSearchPerMinute, measuredAgentPeak)
	}
	if publicReadsPerMinute <= measuredReadPeak {
		t.Errorf("publicReadsPerMinute = %d, must exceed the measured live peak of %d",
			publicReadsPerMinute, measuredReadPeak)
	}
}

// oneShotThrottler admits the first request per key and refuses the rest, so a
// test can exhaust one budget in two calls without depending on the real ceiling.
type oneShotThrottler struct {
	mu   sync.Mutex
	seen map[string]bool
}

func newOneShotThrottler() *oneShotThrottler {
	return &oneShotThrottler{seen: make(map[string]bool)}
}

func (o *oneShotThrottler) Allow(_ context.Context, key string, limit int, window time.Duration) (ratelimit.Decision, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.seen[key] {
		return ratelimit.Decision{Limit: limit, ResetAfter: window, RetryAfter: window}, nil
	}
	o.seen[key] = true
	return ratelimit.Decision{Allowed: true, Limit: limit, Remaining: limit - 1, ResetAfter: window}, nil
}

// A public read is budgeted per IP even for a signed-in caller — the decision this
// records, in place of a comment that claimed the opposite while the code did this.
//
// The gate is mounted FIRST here, unlike production, so an authenticated caller really is
// authenticated by the time the limiter runs. That is what makes this a guard rather than
// a description: with a user-keyed budget it fails whatever the mounting order, and the
// defect it replaces was precisely a claim that only held in one order.
//
// The cost is stated rather than hidden: colleagues behind one office NAT share the
// allowance. They always did — every public read registers its limiter before the
// optional-auth gate, or has no gate at all, so `auth.UserID` never saw a user here.
func TestPublicReadLimiter_BudgetsBySourceAddressEvenForASignedInCaller(t *testing.T) {
	th := newOneShotThrottler()

	app := fiber.New(fiber.Config{ProxyHeader: fiber.HeaderXForwardedFor})
	signIn := func(id int64) fiber.Handler {
		return func(c *fiber.Ctx) error {
			c.Locals(auth.LocalsUserID, id)
			return c.Next()
		}
	}
	ok := func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }
	// Gate before limiter, so the limiter sees a real authenticated caller.
	app.Get("/one", signIn(101), publicReadLimiter(th), ok)
	app.Get("/two", signIn(202), publicReadLimiter(th), ok)

	get := func(path string) int {
		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, path, nil)
		req.Header.Set(fiber.HeaderXForwardedFor, "203.0.113.42")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	if got := get("/one"); got != fiber.StatusOK {
		t.Fatalf("first read = %d, want 200", got)
	}
	if got := get("/two"); got != fiber.StatusTooManyRequests {
		t.Errorf("a different signed-in user on the same address = %d, want 429 — the public read budget is per address", got)
	}
}

// TestPublicReadLimiters_DoNotShareABudget pins that the two classes are keyed
// apart. They exist as two budgets because they cost differently; one shared key
// would quietly undo the split and put facet lookups under the expensive
// endpoint's ceiling.
func TestPublicReadLimiters_DoNotShareABudget(t *testing.T) {
	th := newOneShotThrottler()

	app := fiber.New(fiber.Config{ProxyHeader: fiber.HeaderXForwardedFor})
	app.Get("/cheap", publicReadLimiter(th), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	app.Get("/agent", agentSearchLimiter(th), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	get := func(path string) int {
		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, path, nil)
		req.Header.Set(fiber.HeaderXForwardedFor, "203.0.113.9")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	if got := get("/agent"); got != fiber.StatusOK {
		t.Fatalf("first /agent = %d, want 200", got)
	}
	if got := get("/agent"); got != fiber.StatusTooManyRequests {
		t.Fatalf("second /agent = %d, want 429 (its own budget is spent)", got)
	}
	if got := get("/cheap"); got != fiber.StatusOK {
		t.Errorf("/cheap = %d, want 200 — exhausting the agent budget must not spend the read budget", got)
	}
}

// keyRecorder is a Throttler that refuses everything and keeps the keys it was asked
// about. Refusing is what makes the routes drivable with zero-value handlers: the 429
// is returned by the limiter itself, so no handler body — and no nil dependency — is
// ever reached.
type keyRecorder struct {
	mu   sync.Mutex
	keys []string
}

func (k *keyRecorder) Allow(_ context.Context, key string, limit int, window time.Duration) (ratelimit.Decision, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.keys = append(k.keys, key)
	return ratelimit.Decision{Limit: limit, ResetAfter: window, RetryAfter: window}, nil
}

func (k *keyRecorder) last() (string, bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if len(k.keys) == 0 {
		return "", false
	}
	return k.keys[len(k.keys)-1], true
}

// noAPIKeys satisfies auth.APIKeyAuthenticator for a test that presents only cookies. The
// gates below never reach it: a valid cookie returns before the Bearer branch, and no
// request here carries an Authorization header.
type noAPIKeys struct{}

func (noAPIKeys) AuthenticateAPIKey(context.Context, string) (auth.APIKeyIdentity, error) {
	return auth.APIKeyIdentity{}, errors.New("no such key")
}

// publicReadRoutes mounts every feature register that builds a public-read limiter, each
// on its own app so a param route cannot shadow a literal one, keyed by the handler type
// that owns it. The handlers are zero-valued on purpose — see keyRecorder — and each app
// carries recover so a route reached past its limiter reports the assertion below rather
// than a nil dereference that takes the whole binary down.
//
// The map is hand-written and TestPublicReadLimiters_EveryMountingRegisterIsDriven is what
// keeps it complete: the keys are checked against the receivers the package's own source
// says mount a limiter.
func publicReadRoutes(t *testing.T, throttler ratelimit.Throttler) (map[string]*fiber.App, *auth.Issuer) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	passthrough := func(c *fiber.Ctx) error { return c.Next() }
	mw := middleware{
		optional: auth.OptionalAuth(iss, testVersions, noAPIKeys{}),
		// The moderator writes in jobsHandlers.register are not this test's subject; it
		// drives GET routes only. They still need a handler to be registered at all.
		key:       passthrough,
		moderator: passthrough,
		throttler: throttler,
	}

	mount := func(register func(fiber.Router, middleware)) *fiber.App {
		// ProxyHeader so c.IP() is the address the test states rather than the harness's,
		// which is also how the API reads it behind nginx.
		app := fiber.New(fiber.Config{ProxyHeader: fiber.HeaderXForwardedFor})
		app.Use(recover.New())
		register(app.Group("/api/v1"), mw)
		return app
	}
	return map[string]*fiber.App{
		"jobsHandlers":      mount((&jobsHandlers{}).register),
		"companiesHandlers": mount((&companiesHandlers{}).register),
		"searchHandlers":    mount((&searchHandlers{}).register),
		"suggestHandlers":   mount((&suggestHandlers{}).register),
		"geoHandlers":       mount(newGeoHandlers().register),
	}, iss
}

// publicReadLimiterFuncs are the constructors whose use makes a register this guard's
// business. They are the package's only public-read limiters; every other limiter here
// guards a write, an auth route or an LLM spend, and is keyed by its own rules.
var publicReadLimiterFuncs = []string{"publicReadLimiter", "agentSearchLimiter", "suggestLimiter"}

// TestPublicReadLimiters_EveryMountingRegisterIsDriven derives the scope of the guard
// below from the package's own source, instead of trusting publicReadRoutes' hand-written
// map to be complete.
//
// That map is the same shape as `renderers` one package over, and carries the same trap:
// a sixth register that mounts a public-read limiter is checked by nothing, and the defect
// this whole file exists for would ship again silently. So the expectation is read back
// from the code — every declaration that calls one of the three constructors must be a
// register publicReadRoutes actually drives.
func TestPublicReadLimiters_EveryMountingRegisterIsDriven(t *testing.T) {
	apps, _ := publicReadRoutes(t, &keyRecorder{})

	mounting := limiterMountingOwners(t)
	if len(mounting) == 0 {
		t.Fatal("no declaration in the package mounts a public-read limiter — this guard would pass on anything")
	}
	for _, owner := range mounting {
		if _, ok := apps[owner]; !ok {
			t.Errorf("%s mounts a public-read limiter but publicReadRoutes does not drive it, so nothing "+
				"checks that its key names what the mounted chain can see. Add it to the map.", owner)
		}
	}
}

// limiterMountingOwners reads the package's own non-test sources and names every
// declaration that CALLS one of publicReadLimiterFuncs: the receiver type for a method,
// the function's own name otherwise. A limiter mounted outside a handler type therefore
// names something that cannot be in the map, which is the right answer — the guard drives
// registers, and a mount that is not one needs a human to decide what holds it.
//
// The declarations of the constructors themselves do not match: they build the middleware
// from ratelimit.Middleware and never call each other.
//
// `go test` runs with the package directory as the working directory, so the sources are
// simply "." — no module root to resolve.
func limiterMountingOwners(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package directory: %v", err)
	}

	var owners []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !mountsAPublicReadLimiter(fn.Body) {
				continue
			}
			owner := fn.Name.Name
			if fn.Recv != nil && len(fn.Recv.List) == 1 {
				owner = receiverTypeName(fn.Recv.List[0].Type)
			}
			if !slices.Contains(owners, owner) {
				owners = append(owners, owner)
			}
		}
	}
	return owners
}

// mountsAPublicReadLimiter reports whether a function body calls one of the constructors
// by its bare name, which is how a same-package call reads.
func mountsAPublicReadLimiter(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok && slices.Contains(publicReadLimiterFuncs, ident.Name) {
			found = true
			return false
		}
		return true
	})
	return found
}

// receiverTypeName is the bare type name of a receiver, pointer or value. A form this does
// not recognise reports "?", which matches no map key and so fails loudly.
func receiverTypeName(expr ast.Expr) string {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return "?"
}

// TestPublicReadLimiters_KeyWhatTheMountedChainCanSee drives the real registrations,
// which is the whole point: the key function was correct in isolation and wrong in
// place, and the isolated tests could not tell — they mount the limiter on a bare app,
// where the handler order under test is the test's own.
//
// The rule it holds is the one in AGENTS.md. Two signed-in callers at the same address
// request each route; either the limiter can tell them apart, in which case an
// authentication gate ran before it and the key must name the user, or it cannot, in
// which case the key must be exactly the route's namespace and the address — with no
// ":user:"/":ip:" discriminator asserting a distinction the chain never makes.
func TestPublicReadLimiters_KeyWhatTheMountedChainCanSee(t *testing.T) {
	const clientIP = "203.0.113.9"

	rec := &keyRecorder{}
	apps, iss := publicReadRoutes(t, rec)

	keyFor := func(t *testing.T, app *fiber.App, path string, userID int64) string {
		t.Helper()
		token, err := iss.Issue(userID, testTokenVersion)
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, path, nil)
		req.Header.Set(fiber.HeaderXForwardedFor, clientIP)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusTooManyRequests {
			// 500 here is the shape of a GET that carries no limiter at all: the request
			// reached a zero-valued handler and recover turned the nil dereference into a
			// status. Either way the route is not throttled, which is the finding.
			t.Fatalf("GET %s = %d, want 429 — the limiter must be the first handler on the chain",
				path, resp.StatusCode)
		}
		key, ok := rec.last()
		if !ok {
			t.Fatalf("GET %s: the throttler was never consulted", path)
		}
		return key
	}

	checked := 0
	for feature, app := range apps {
		for _, route := range app.GetRoutes(true) {
			if route.Method != fiber.MethodGet {
				continue
			}
			path := route.Path
			for _, p := range route.Params {
				path = strings.Replace(path, ":"+p, "a-sample-slug", 1)
			}
			t.Run(feature+" "+route.Path, func(t *testing.T) {
				first := keyFor(t, app, path, 7)
				second := keyFor(t, app, path, 8)
				checked++

				if first != second {
					// A gate ran ahead of the limiter, so a user-keyed limiter is right here —
					// and it must actually carry the caller, not merely differ.
					if !strings.Contains(first, strconv.Itoa(7)) || !strings.Contains(second, strconv.Itoa(8)) {
						t.Errorf("keys differ per caller but name neither: %q / %q", first, second)
					}
					return
				}

				namespace, rest, found := strings.Cut(first, ":")
				if !found {
					t.Fatalf("key %q carries no route namespace", first)
				}
				if rest != clientIP {
					t.Errorf("%s keys every signed-in caller alike, so its key must be %q — it is %q. "+
						"A ':user:'/':ip:' key here claims a distinction the chain cannot make: the limiter "+
						"is mounted before any authentication gate. Move the gate ahead of it, or use "+
						"ratelimit.KeyByIP.", route.Path, namespace+":"+clientIP, first)
				}
			})
		}
	}
	if checked == 0 {
		t.Fatal("no public read route was driven, so this guard proved nothing")
	}
}

// TestPublicReadLimiters_TheSessionCookieIsRealAndArrivesTooLate is the other half of the
// test above: it proves the cookie those requests carry does authenticate, so the
// limiter's blindness is about ORDER and not about a credential the test got wrong.
func TestPublicReadLimiters_TheSessionCookieIsRealAndArrivesTooLate(t *testing.T) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	app := fiber.New()
	app.Get("/probe", auth.OptionalAuth(iss, testVersions, noAPIKeys{}), func(c *fiber.Ctx) error {
		id, ok := auth.UserID(c)
		if !ok {
			return c.SendString("anonymous")
		}
		return c.SendString(strconv.FormatInt(id, 10))
	})

	token, err := iss.Issue(7, testTokenVersion)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/probe", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("GET /probe: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if got := string(body); got != "7" {
		t.Fatalf("behind the gate the caller reads as %q, want \"7\" — the cookie the sibling test "+
			"sends would not have identified anybody either", got)
	}
}
