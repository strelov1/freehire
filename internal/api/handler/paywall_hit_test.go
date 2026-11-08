package handler

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/identity/auth"
)

const testUUID = "00000000-0000-0000-0000-000000000001"

// wantPaywallHit is what a free caller's refusal on a gated route must record.
type wantPaywallHit struct {
	feature string
	job     plan.PaywallJob
}

var cvUUID = uuid.MustParse(testUUID)

// paywallHitFor holds the record every route in gatedRoutes writes, keyed by "METHOD path".
func paywallHitFor(r gatedRoute) wantPaywallHit {
	feature := string(r.feature)
	job := plan.PaywallJob{}
	switch {
	case r.path == "/api/v1/me/profile/ats-report":
		feature = "ats_review"
	case strings.HasSuffix(r.path, "/mail-recall"):
		feature, job = "mail_recall", plan.PaywallJob{Slug: "some-job"}
	case r.path == "/api/v1/me/autofill/run":
		feature = "autofill"
	case strings.Contains(r.path, "/assistant/sessions/"):
		job = plan.PaywallJob{SessionID: cvUUID}
	case strings.Contains(r.path, "/me/cvs/"+testUUID):
		job = plan.PaywallJob{CVID: cvUUID}
	case strings.HasPrefix(r.path, "/api/v1/jobs/"):
		job = plan.PaywallJob{Slug: "some-job"}
	}
	return wantPaywallHit{feature: feature, job: job}
}

func TestEveryGatedRouteRecordsTheFreeCallersRefusal(t *testing.T) {
	for _, r := range gatedRoutes {
		plans := &fakeEntitlement{tier: plan.TierFree}
		status, _, body := send(t, gatedApp(t, plans), r.method, r.path)
		if status != fiber.StatusPaymentRequired {
			t.Errorf("%s %s: status %d (%s), want 402", r.method, r.path, status, body)
			continue
		}
		want := paywallHitFor(r)
		hits := plans.recorded()
		if len(hits) != 1 {
			t.Errorf("%s %s recorded %d hits, want 1", r.method, r.path, len(hits))
			continue
		}
		if got := hits[0]; got.userID != 7 || got.feature != want.feature || got.job != want.job {
			t.Errorf("%s %s recorded %+v, want user 7, feature %q, job %+v", r.method, r.path, got, want.feature, want.job)
		}
	}
}

// A route with no allowance of its own is named from a fixed table; one missing from it must
// not leak into the record under an empty or URL-derived name.
func TestEveryUnmeteredRouteHasAFixedPaywallName(t *testing.T) {
	for _, r := range gatedRoutes {
		if r.feature != unmeteredAI {
			continue
		}
		plans := &fakeEntitlement{tier: plan.TierFree}
		send(t, gatedApp(t, plans), r.method, r.path)
		hits := plans.recorded()
		if len(hits) != 1 || hits[0].feature == "" || hits[0].feature == unknownPaywallFeature {
			t.Errorf("%s %s recorded %+v, want a named unmetered feature", r.method, r.path, hits)
		}
	}
}

func TestTailorRecordsTheJobSlugFromTheBody(t *testing.T) {
	plans := &fakeEntitlement{tier: plan.TierFree}
	app := gatedApp(t, plans)
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/me/cvs/tailor",
		strings.NewReader(`{"job_slug":"body-job"}`))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusPaymentRequired {
		t.Fatalf("status %d, want 402", resp.StatusCode)
	}
	hits := plans.recorded()
	if len(hits) != 1 || hits[0].feature != "tailor" || hits[0].job.Slug != "body-job" {
		t.Fatalf("recorded %+v, want one tailor hit for body-job", hits)
	}
}

// A path value that is not an id is no reference, never an error.
func TestAMalformedIDRecordsNoJob(t *testing.T) {
	plans := &fakeEntitlement{tier: plan.TierFree}
	status, _, _ := send(t, gatedApp(t, plans), fiber.MethodPost, "/api/v1/me/cvs/not-a-uuid/cover-letter")
	if status != fiber.StatusPaymentRequired {
		t.Fatalf("status %d, want 402", status)
	}
	hits := plans.recorded()
	if len(hits) != 1 || hits[0].job != (plan.PaywallJob{}) {
		t.Fatalf("recorded %+v, want one hit without a job", hits)
	}
}

func TestPaidCallersAndUnreadablePlansRecordNothing(t *testing.T) {
	for name, plans := range map[string]*fakeEntitlement{
		"pro":        {tier: plan.TierPro},
		"ultra":      {tier: plan.TierUltra},
		"unreadable": {err: errors.New("database is down")},
	} {
		app := gatedApp(t, plans)
		for _, r := range gatedRoutes {
			send(t, app, r.method, r.path)
		}
		if hits := plans.recorded(); len(hits) != 0 {
			t.Errorf("%s: recorded %+v, want nothing", name, hits)
		}
	}
}

func TestAMissingPlanStoreRecordsNothing(t *testing.T) {
	status, _, _ := send(t, gatedApp(t, nil), fiber.MethodPost, "/api/v1/jobs/some-job/fit")
	if status != fiber.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", status)
	}
}

// The 402 is the same whether the record lands, fails or panics, and a failure is logged.
func TestAFailedRecordLeavesTheRefusalUnchanged(t *testing.T) {
	const route = "/api/v1/jobs/some-job/fit"
	_, _, wantBody := send(t, gatedApp(t, &fakeEntitlement{tier: plan.TierFree}), fiber.MethodPost, route)

	var logged bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(prev) })

	plans := &fakeEntitlement{tier: plan.TierFree, recordErr: errors.New("connection reset")}
	status, _, body := send(t, gatedApp(t, plans), fiber.MethodPost, route)
	paywallWrites.Wait()
	if status != fiber.StatusPaymentRequired || body != wantBody {
		t.Errorf("a failed record changed the refusal: %d %s, want 402 %s", status, body, wantBody)
	}
	if !strings.Contains(logged.String(), "connection reset") {
		t.Errorf("the failed write was not logged: %q", logged.String())
	}
}

func TestAPanickingRecordDoesNotCrashTheRefusal(t *testing.T) {
	var logged bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(prev) })

	plans := &fakeEntitlement{tier: plan.TierFree, recordPanic: true}
	status, _, _ := send(t, gatedApp(t, plans), fiber.MethodPost, "/api/v1/jobs/some-job/fit")
	paywallWrites.Wait()
	if status != fiber.StatusPaymentRequired {
		t.Errorf("status %d, want 402", status)
	}
	if !strings.Contains(logged.String(), "record exploded") {
		t.Errorf("the panic was not logged: %q", logged.String())
	}
}

// The write outlives the request: it must not be cancelled with it.
func TestTheRecordSurvivesTheRequestContext(t *testing.T) {
	plans := &fakeEntitlement{tier: plan.TierFree}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/x", func(c *fiber.Ctx) error {
		c.Locals(auth.LocalsUserID, int64(7))
		ctx, cancel := context.WithCancel(c.UserContext())
		c.SetUserContext(ctx)
		cancel()
		return c.Next()
	}, paidAIGate(plans)(plan.FeatureFit))
	if status, _, _ := send(t, app, fiber.MethodPost, "/x"); status != fiber.StatusPaymentRequired {
		t.Fatalf("status %d, want 402", status)
	}
	if hits := plans.recorded(); len(hits) != 1 {
		t.Fatalf("recorded %d hits after the request context was cancelled, want 1", len(hits))
	}
}

// The orchestrator acts for a queue owner, not for someone who pressed a button.
func TestTheUnrecordedGateRefusesWithoutRecording(t *testing.T) {
	plans := &fakeEntitlement{tier: plan.TierFree}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/x", func(c *fiber.Ctx) error {
		ok, err := authorizePaidAIUnrecorded(c, plans, 7, plan.FeatureTailor)
		if ok {
			return c.SendStatus(fiber.StatusNoContent)
		}
		return err
	})
	if status, _, _ := send(t, app, fiber.MethodPost, "/x"); status != fiber.StatusPaymentRequired {
		t.Fatalf("status %d, want 402", status)
	}
	if hits := plans.recorded(); len(hits) != 0 {
		t.Fatalf("recorded %+v, want nothing", hits)
	}
}
