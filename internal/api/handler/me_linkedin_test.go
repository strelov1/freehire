package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/ratelimit"
	"github.com/strelov1/freehire/internal/candidate/linkedinprofile"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// stubReader stands in for the network. What the reader does with a page is settled in
// linkedinprofile's own tests; what matters here is that the handler asks it at the right
// moments, never at the wrong ones, and renders each of its three outcomes distinctly.
type stubReader struct {
	profile linkedinprofile.Profile
	err     error
	calls   int
	gotURL  string
}

func (s *stubReader) Fetch(_ context.Context, input string) (linkedinprofile.Profile, error) {
	s.calls++
	s.gotURL = input
	return s.profile, s.err
}

func linkedInApp(t *testing.T, reader *stubReader) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &linkedInHandlers{reader: reader}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/me/linkedin/import", auth.RequireAuth(iss, testVersions), h.ImportLinkedInProfile)
	return app, token
}

func postLinkedIn(t *testing.T, app *fiber.App, body, token string) *http.Response {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/me/linkedin/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	return resp
}

func decodeData(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var out struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	return out.Data
}

// The whole point of the endpoint: a headline becomes the same facets a CV carrying the
// same words would, because it goes through the same helper.
func TestImportLinkedInProfileDerivesFacetsFromTheHeadline(t *testing.T) {
	t.Parallel()

	reader := &stubReader{profile: linkedinprofile.Profile{
		Name:     "Dana Okonkwo",
		Headline: "Senior Backend Engineer working in TypeScript/Node.js and Python",
		Location: "Florianópolis, Santa Catarina, Brazil",
		Company:  "Northwind Systems",
	}}
	app, token := linkedInApp(t, reader)

	resp := postLinkedIn(t, app, `{"url":"https://www.linkedin.com/in/danaokonkwo"}`, token)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	data := decodeData(t, resp)

	if got := data["seniority"]; got != "senior" {
		t.Errorf("seniority = %v, want senior", got)
	}
	if !containsString(data["categories"], "backend") {
		t.Errorf("categories = %v, want it to include backend", data["categories"])
	}
	for _, want := range []string{"typescript", "nodejs", "python"} {
		if !containsString(data["skills"], want) {
			t.Errorf("skills = %v, want it to include %q", data["skills"], want)
		}
	}

	// The display fields are what lets the UI say "here is what we recognised" rather
	// than silently changing the form under the user.
	if data["name"] != "Dana Okonkwo" || data["company"] != "Northwind Systems" {
		t.Errorf("display fields = %v / %v", data["name"], data["company"])
	}

	loc, ok := data["location"].(map[string]any)
	if !ok {
		t.Fatalf("location = %v, want an object", data["location"])
	}
	if !containsString(loc["countries"], "br") {
		t.Errorf("countries = %v, want br", loc["countries"])
	}
	if !containsString(loc["regions"], "latam") {
		t.Errorf("regions = %v, want latam", loc["regions"])
	}
}

// Same wire shape as /me/resume/extract, because the client merges both into one staged
// set: skills and categories always arrays, seniority absent rather than empty when the
// dictionaries resolved none.
func TestImportLinkedInProfileMatchesTheCVExtractShape(t *testing.T) {
	t.Parallel()

	reader := &stubReader{profile: linkedinprofile.Profile{
		Name:     "Dana Okonkwo",
		Headline: "Helping companies grow",
	}}
	app, token := linkedInApp(t, reader)

	resp := postLinkedIn(t, app, `{"url":"danaokonkwo"}`, token)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 — an unrecognised headline is not an error", resp.StatusCode)
	}
	data := decodeData(t, resp)

	if _, isArray := data["skills"].([]any); !isArray {
		t.Errorf("skills = %#v, want an array even when empty", data["skills"])
	}
	if _, isArray := data["categories"].([]any); !isArray {
		t.Errorf("categories = %#v, want an array even when empty", data["categories"])
	}
	if _, present := data["seniority"]; present {
		t.Error("seniority is present but unresolved — a client would read the empty string as a value")
	}
	if _, present := data["location"]; present {
		t.Error("location is present but the profile carried no address")
	}
}

func TestImportLinkedInProfileRendersEachOutcomeDistinctly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"a URL that names no profile", linkedinprofile.ErrNotAProfileURL, fiber.StatusBadRequest},
		{"a page that did not arrive", linkedinprofile.ErrFetch, fiber.StatusBadGateway},
		{"a page that said nothing", linkedinprofile.ErrNoProfile, fiber.StatusUnprocessableEntity},
		// An error the reader is not documented to return is a bug on our side, and must
		// reach RenderError as one: telling the user LinkedIn did not answer would be a
		// lie, and would keep the bug off the error tracker for as long as it lived.
		{"something else entirely", fmt.Errorf("boom"), fiber.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app, token := linkedInApp(t, &stubReader{err: tt.err})
			resp := postLinkedIn(t, app, `{"url":"danaokonkwo"}`, token)
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			_ = resp.Body.Close()
		})
	}
}

// An outbound fetch of a third-party page must never be something an anonymous caller can
// cause.
func TestImportLinkedInProfileRefusesAnonymousBeforeFetching(t *testing.T) {
	t.Parallel()

	reader := &stubReader{}
	app, _ := linkedInApp(t, reader)

	resp := postLinkedIn(t, app, `{"url":"danaokonkwo"}`, "")
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	_ = resp.Body.Close()
	if reader.calls != 0 {
		t.Errorf("an anonymous request caused %d outbound fetches", reader.calls)
	}
}

// An empty URL is rejected here rather than handed on, so a stray submit costs nothing.
func TestImportLinkedInProfileRejectsAnEmptyURLBeforeFetching(t *testing.T) {
	t.Parallel()

	for _, body := range []string{`{"url":""}`, `{"url":"   "}`, `{}`, `not json`} {
		reader := &stubReader{}
		app, token := linkedInApp(t, reader)
		resp := postLinkedIn(t, app, body, token)
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Errorf("body %q: status = %d, want 400", body, resp.StatusCode)
		}
		_ = resp.Body.Close()
		if reader.calls != 0 {
			t.Errorf("body %q caused %d outbound fetches", body, reader.calls)
		}
	}
}

// The handler hands the reader what the user pasted, untouched — validating a URL is the
// reader's job, and doing it twice in two places is how the two answers drift apart.
func TestImportLinkedInProfilePassesTheInputThrough(t *testing.T) {
	t.Parallel()

	reader := &stubReader{profile: linkedinprofile.Profile{Name: "Dana Okonkwo"}}
	app, token := linkedInApp(t, reader)

	resp := postLinkedIn(t, app, `{"url":"  https://br.linkedin.com/in/danaokonkwo?trk=x  "}`, token)
	_ = resp.Body.Close()

	if reader.gotURL != "https://br.linkedin.com/in/danaokonkwo?trk=x" {
		t.Errorf("reader got %q", reader.gotURL)
	}
}

// TestLinkedInRegister_MountsCookieThenOutboundFetch pins the real register() rather than a
// replica of it.
//
// Both guarantees the spec makes about this route are properties of the mounting, not of the
// handler: an anonymous caller must be refused before any outbound request, and a throttled
// one likewise. A test that builds its own middleware chain proves the handler behaves once
// those gates are in place, and proves nothing at all about whether they are — a dropped or
// reordered gate here would pass every other test in this file.
//
// The order matters twice over: mw.cookie must come first so the throttler's key resolves to
// the user rather than falling back to the address, and both must come before the handler so
// a refused request costs LinkedIn nothing.
func TestLinkedInRegister_MountsCookieThenOutboundFetch(t *testing.T) {
	t.Parallel()

	var order []string
	record := func(name string) fiber.Handler {
		return func(c *fiber.Ctx) error {
			order = append(order, name)
			return c.Next()
		}
	}

	reader := &stubReader{profile: linkedinprofile.Profile{Name: "Dana Okonkwo"}}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	api := app.Group("/api/v1")
	(&linkedInHandlers{reader: reader}).register(api, middleware{
		cookie:        record("cookie"),
		outboundFetch: record("outboundFetch"),
		key:           record("key"),
	})

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost,
		"/api/v1/me/linkedin/import", strings.NewReader(`{"url":"danaokonkwo"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	_ = resp.Body.Close()

	if want := []string{"cookie", "outboundFetch"}; !slices.Equal(order, want) {
		t.Fatalf("route ran %v, want %v", order, want)
	}
	if reader.calls != 1 {
		t.Errorf("handler ran %d times, want 1", reader.calls)
	}
}

// A route that stopped requiring a session would be an outbound-fetch amplifier anyone could
// aim. This holds the real chain, with the real cookie gate, against that.
func TestLinkedInRegister_RefusesAnonymousWithTheRealChain(t *testing.T) {
	t.Parallel()

	iss := auth.NewIssuer("test-secret", time.Hour)
	reader := &stubReader{}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	api := app.Group("/api/v1")
	(&linkedInHandlers{reader: reader}).register(api, middleware{
		cookie:        auth.RequireAuth(iss, testVersions),
		outboundFetch: func(c *fiber.Ctx) error { return c.Next() },
	})

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost,
		"/api/v1/me/linkedin/import", strings.NewReader(`{"url":"danaokonkwo"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if reader.calls != 0 {
		t.Errorf("an anonymous request caused %d outbound fetches", reader.calls)
	}
}

// denyThrottler refuses everything, standing in for a user who has spent the shared
// outbound-fetch budget.
type denyThrottler struct{}

func (denyThrottler) Allow(context.Context, string, int, time.Duration) (ratelimit.Decision, error) {
	return ratelimit.Decision{Allowed: false, Limit: 20, RetryAfter: time.Minute}, nil
}

// A throttled call must cost LinkedIn nothing. The limiter runs as middleware ahead of the
// handler, so the guarantee is one of ordering — and ordering is exactly the kind of thing
// that survives a refactor only if a test holds it down.
func TestImportLinkedInProfileThrottledMakesNoOutboundFetch(t *testing.T) {
	t.Parallel()

	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	reader := &stubReader{profile: linkedinprofile.Profile{Name: "Dana Okonkwo"}}
	h := &linkedInHandlers{reader: reader}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	limiter := ratelimit.Middleware(denyThrottler{}, ratelimit.KeyByUserOrIP("linkedin-import"), 20, time.Hour)
	app.Post("/me/linkedin/import", auth.RequireAuth(iss, testVersions), limiter, h.ImportLinkedInProfile)

	resp := postLinkedIn(t, app, `{"url":"danaokonkwo"}`, token)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != fiber.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", resp.StatusCode)
	}
	if reader.calls != 0 {
		t.Errorf("a throttled request caused %d outbound fetches", reader.calls)
	}
}

// The import must not create a CV, because CV presence is the onboarding page's own redirect
// gate: an import that quietly satisfied it would stop prompting a user who still has no CV.
//
// This is held structurally rather than by observing a store — linkedInHandlers is given no
// résumé store, no profile service and no queries, so there is nothing for it to write
// through. The assertion is that the type stays that way: adding a writer to it is exactly
// the change that would break the gate, and it should have to break this test first.
func TestLinkedInHandlersHoldNothingItCouldWriteThrough(t *testing.T) {
	t.Parallel()

	fields := reflect.VisibleFields(reflect.TypeOf(linkedInHandlers{}))
	if len(fields) != 1 || fields[0].Name != "reader" {
		var names []string
		for _, f := range fields {
			names = append(names, f.Name)
		}
		t.Fatalf("linkedInHandlers holds %v; it may hold only the reader, or the import is no "+
			"longer guaranteed to persist nothing", names)
	}
}

func containsString(v any, want string) bool {
	items, ok := v.([]any)
	if !ok {
		return false
	}
	for _, it := range items {
		if s, ok := it.(string); ok && s == want {
			return true
		}
	}
	return false
}
