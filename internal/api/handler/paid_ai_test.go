package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// fakeEntitlement answers the paid-AI gate's questions without a database, counting how
// often it was asked so a test can prove a route never consulted it.
type fakeEntitlement struct {
	tier  plan.Tier
	err   error
	asked atomic.Int32

	// recordErr and recordPanic make the paywall-hit write fail or panic; hits collects what
	// the gate asked to record.
	recordErr   error
	recordPanic bool
	mu          sync.Mutex
	hits        []recordedHit
}

type recordedHit struct {
	userID  int64
	feature string
	job     plan.PaywallJob
}

func (f *fakeEntitlement) RecordPaywallHit(_ context.Context, userID int64, feature string, job plan.PaywallJob) error {
	if f.recordPanic {
		panic("record exploded")
	}
	if f.recordErr != nil {
		return f.recordErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hits = append(f.hits, recordedHit{userID, feature, job})
	return nil
}

// recorded waits for every detached paywall-hit write to finish and returns what they stored.
func (f *fakeEntitlement) recorded() []recordedHit {
	paywallWrites.Wait()
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedHit(nil), f.hits...)
}

func (f *fakeEntitlement) RequirePaid(context.Context, int64) (plan.Tier, error) {
	f.asked.Add(1)
	if f.err != nil {
		return "", f.err
	}
	if !f.tier.Paid() {
		return f.tier, plan.ErrPaymentRequired
	}
	return f.tier, nil
}

func (f *fakeEntitlement) Standing(_ context.Context, _ int64, feature plan.Feature) (plan.Standing, error) {
	a := plan.DefaultConfig().Allowance(f.tier, feature)
	return plan.Standing{Tier: f.tier, Feature: feature, Limit: a.Limit, Unlimited: a.Unlimited, Enforced: true}, nil
}

// gatedRoute is one route that makes a model call for its caller, with the feature its
// refusal must name (the zero Feature for an AI action with no allowance of its own).
type gatedRoute struct {
	method, path string
	feature      plan.Feature
}

// gatedRoutes is every user-initiated model call this API serves. Adding a route that
// calls a model means adding it here — and mounting mw.paidAI on it, or the first test
// below fails.
var gatedRoutes = []gatedRoute{
	{fiber.MethodPost, "/api/v1/jobs/some-job/referral-message/stream", plan.FeatureCoverLetter},
	{fiber.MethodPost, "/api/v1/jobs/some-job/fit", plan.FeatureFit},
	{fiber.MethodGet, "/api/v1/jobs/some-job/fit/stream", plan.FeatureFit},
	{fiber.MethodPost, "/api/v1/jobs/some-job/match-analysis", plan.FeatureFit},
	{fiber.MethodGet, "/api/v1/jobs/some-job/match-analysis/stream", plan.FeatureFit},
	{fiber.MethodPost, "/api/v1/assistant/sessions/00000000-0000-0000-0000-000000000001/messages", plan.FeatureAssistant},
	{fiber.MethodPost, "/api/v1/assistant/sessions/00000000-0000-0000-0000-000000000001/retry", plan.FeatureAssistant},
	{fiber.MethodPost, "/api/v1/assistant/sessions/00000000-0000-0000-0000-000000000001/opening", plan.FeatureAssistant},
	{fiber.MethodPost, "/api/v1/assistant/sessions/00000000-0000-0000-0000-000000000001/voice-token", plan.FeatureAssistant},
	{fiber.MethodPost, "/api/v1/assistant/sessions/00000000-0000-0000-0000-000000000001/autopilot", plan.FeatureTailor},
	{fiber.MethodPost, "/api/v1/jobs/some-job/auto-apply", plan.FeatureAutoApply},
	{fiber.MethodPost, "/api/v1/me/cvs/tailor", plan.FeatureTailor},
	{fiber.MethodPost, "/api/v1/me/cvs/00000000-0000-0000-0000-000000000001/tailor-session", plan.FeatureTailor},
	{fiber.MethodPost, "/api/v1/me/cvs/00000000-0000-0000-0000-000000000001/cover-letter", plan.FeatureCoverLetter},
	{fiber.MethodPost, "/api/v1/me/cvs/00000000-0000-0000-0000-000000000001/cover-letter/stream", plan.FeatureCoverLetter},
	{fiber.MethodPost, "/api/v1/speech/transcriptions", plan.FeatureDictation},
	{fiber.MethodPost, "/api/v1/me/profile/ats-report", unmeteredAI},
	{fiber.MethodPost, "/api/v1/me/tracking/some-job/mail-recall", unmeteredAI},
	{fiber.MethodPost, "/api/v1/me/autofill/run", unmeteredAI},
}

// openRoutes read a cached AI result, compute something deterministic, or make one of the
// model calls every account gets (plan.WithFreeAI) — the CV parse and the search
// interpretation. They must stay reachable to free, so the gate must never even be asked.
var openRoutes = []gatedRoute{
	{method: fiber.MethodGet, path: "/api/v1/jobs/some-job/fit"},
	{method: fiber.MethodGet, path: "/api/v1/jobs/some-job/match-analysis"},
	{method: fiber.MethodGet, path: "/api/v1/jobs/some-job/match"},
	{method: fiber.MethodGet, path: "/api/v1/me/profile/ats-report"},
	{method: fiber.MethodGet, path: "/api/v1/me/resume"},
	{method: fiber.MethodPut, path: "/api/v1/me/resume"},
	{method: fiber.MethodPost, path: "/api/v1/me/resume/extract"},
	{method: fiber.MethodPost, path: "/api/v1/me/resume/retry-extract"},
	{method: fiber.MethodGet, path: "/api/v1/me/cvs/00000000-0000-0000-0000-000000000001/cover-letter"},
	{method: fiber.MethodGet, path: "/api/v1/me/cvs/00000000-0000-0000-0000-000000000001/tailor-context"},
	{method: fiber.MethodPost, path: "/api/v1/cv/roast"},
	{method: fiber.MethodPost, path: "/api/v1/search/interpret"},
}

// gatedApp registers every feature that serves a model call, as production does, over
// zero-valued handlers: the gate must answer before any of them is reached. Every
// authentication gate stands in as "signed in as user 7".
func gatedApp(t *testing.T, plans paidEntitlement) *fiber.App {
	t.Helper()
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	// A zero handler that IS reached panics on its nil dependencies. Recovered into a 500
	// so a test can tell "the gate let it through" from "the gate stopped it".
	app.Use(recover.New())
	signedIn := func(c *fiber.Ctx) error {
		c.Locals(auth.LocalsUserID, int64(7))
		return c.Next()
	}
	mw := middleware{
		key: signedIn, cvKey: signedIn, cookie: signedIn, optional: signedIn,
		autoApplyGate: signedIn,
		paidAI:        paidAIGate(plans),
	}
	api := app.Group("/api/v1")
	(&matchHandlers{}).register(api, mw)
	(&assistantHandlers{}).register(api, mw)
	(&cvHandlers{}).register(api, mw, cvContract(&cvHandlers{}))
	(&referralMessageHandlers{}).register(api, mw)
	resumeH := &resumeHandlers{}
	resumeH.register(api, mw, newContract(contractHandlers{resumeHandlers: resumeH}))
	(&speechHandlers{}).register(api, mw)
	(&intentHandlers{}).register(api, mw)
	(&inboxHandlers{}).register(api, mw)
	(&autofillHandlers{}).register(api, mw)
	return app
}

func send(t *testing.T, app *fiber.App, method, path string) (int, string, string) {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader("{}"))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get(fiber.HeaderContentType), string(body)
}

func TestEveryModelCallRouteRefusesFreeBeforeItRuns(t *testing.T) {
	app := gatedApp(t, &fakeEntitlement{tier: plan.TierFree})
	for _, r := range gatedRoutes {
		status, ctype, body := send(t, app, r.method, r.path)
		if status != fiber.StatusPaymentRequired {
			t.Errorf("%s %s as free: status %d (%s), want 402 — the route makes a model call without the paid gate",
				r.method, r.path, status, body)
			continue
		}
		// A 402 rather than a stream: the refusal is written before any SSE header goes out.
		if strings.HasPrefix(ctype, "text/event-stream") {
			t.Errorf("%s %s: the refusal arrived as a stream", r.method, r.path)
		}
		var got struct {
			Error      string `json:"error"`
			UpgradeURL string `json:"upgrade_url"`
			Allowance  *struct {
				Feature string `json:"feature"`
			} `json:"allowance"`
		}
		if err := json.Unmarshal([]byte(body), &got); err != nil {
			t.Fatalf("%s %s: refusal body %q is not JSON: %v", r.method, r.path, body, err)
		}
		if got.Error == "" || got.UpgradeURL != upgradePath {
			t.Errorf("%s %s: refusal body %s lacks the message or the upgrade link the clients render", r.method, r.path, body)
		}
		switch {
		case r.feature == unmeteredAI && got.Allowance != nil:
			t.Errorf("%s %s: an unmetered refusal quoted an allowance: %s", r.method, r.path, body)
		case r.feature != unmeteredAI && (got.Allowance == nil || got.Allowance.Feature != string(r.feature)):
			t.Errorf("%s %s: refusal does not name %q: %s", r.method, r.path, r.feature, body)
		}
	}
}

// A plan that cannot be read never authorises a model call, and is not an upgrade prompt.
func TestAnUnreadablePlanIs503NotAModelCall(t *testing.T) {
	app := gatedApp(t, &fakeEntitlement{err: errors.New("database is down")})
	for _, r := range gatedRoutes {
		if status, _, body := send(t, app, r.method, r.path); status != fiber.StatusServiceUnavailable {
			t.Errorf("%s %s with an unreadable plan: status %d (%s), want 503", r.method, r.path, status, body)
		}
	}
}

// A paying caller passes the gate. Here that means reaching the zero handler behind it —
// a 500 from the recovered panic, or whatever it answers first — rather than a 402 or 503.
func TestPaidCallersPassTheGate(t *testing.T) {
	for _, tier := range []plan.Tier{plan.TierPro, plan.TierUltra} {
		app := gatedApp(t, &fakeEntitlement{tier: tier})
		for _, r := range gatedRoutes {
			status, _, body := send(t, app, r.method, r.path)
			if status == fiber.StatusPaymentRequired || status == fiber.StatusServiceUnavailable && strings.Contains(body, "plan information") {
				t.Errorf("%s %s as %s: status %d (%s), want the gate to let it through", r.method, r.path, tier, status, body)
			}
		}
	}
}

// Cached AI results and deterministic paths stay open to free: the gate is never asked.
func TestCachedAndDeterministicRoutesNeverAskTheGate(t *testing.T) {
	plans := &fakeEntitlement{tier: plan.TierFree}
	app := gatedApp(t, plans)
	for _, r := range openRoutes {
		before := plans.asked.Load()
		status, _, body := send(t, app, r.method, r.path)
		if plans.asked.Load() != before || status == fiber.StatusPaymentRequired {
			t.Errorf("%s %s consulted the paid gate (status %d, %s); it makes no model call and must stay open to free",
				r.method, r.path, status, body)
		}
	}
}

// The gate is what marks the request; the binding is what reads the mark.
func TestTheGateMarksThePaidRequestForTheBinding(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	sawPaid := false
	app.Post("/x", func(c *fiber.Ctx) error {
		c.Locals(auth.LocalsUserID, int64(7))
		return c.Next()
	}, paidAIGate(&fakeEntitlement{tier: plan.TierPro})(unmeteredAI), func(c *fiber.Ctx) error {
		sawPaid = plan.PaidAI(c.UserContext())
		return c.SendStatus(fiber.StatusNoContent)
	})
	if status, _, _ := send(t, app, fiber.MethodPost, "/x"); status != fiber.StatusNoContent || !sawPaid {
		t.Fatalf("status %d, paid mark seen %v — the gate passed without marking the request", status, sawPaid)
	}
}
