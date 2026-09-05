package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/promo"
)

// stubPromoRepo is the discount tables, in memory. It counts the reads that cost something
// so a test can assert a refusal happened before the database was touched.
type stubPromoRepo struct {
	usable   map[string]int16
	redeemed map[int64]bool
	previews int
	redeems  int
}

func (s *stubPromoRepo) PreviewCode(_ context.Context, code string) (int16, error) {
	s.previews++
	if pct, ok := s.usable[code]; ok {
		return pct, nil
	}
	return 0, promo.ErrNotUsable
}

func (s *stubPromoRepo) Redeem(_ context.Context, userID int64, code string) (int16, error) {
	s.redeems++
	pct, ok := s.usable[code]
	if !ok || s.redeemed[userID] {
		return 0, promo.ErrNotUsable
	}
	s.redeemed[userID] = true
	return pct, nil
}

func (s *stubPromoRepo) HasRedeemed(_ context.Context, userID int64) (bool, error) {
	return s.redeemed[userID], nil
}
func (s *stubPromoRepo) RedeemedPercent(context.Context, int64) (int16, error) { return 0, nil }
func (s *stubPromoRepo) EnsureInviteCode(_ context.Context, _ int64, code string) (string, error) {
	return code, nil
}
func (s *stubPromoRepo) ReferrerByCode(context.Context, string) (int64, error) {
	return 0, promo.ErrNotUsable
}
func (s *stubPromoRepo) Attribute(context.Context, int64, int64) (bool, error) { return false, nil }
func (s *stubPromoRepo) Stats(context.Context, int64) (promo.Stats, error) {
	return promo.Stats{}, nil
}
func (s *stubPromoRepo) HasPendingInvite(context.Context, int64) (bool, error) { return false, nil }
func (s *stubPromoRepo) PendingRewards(context.Context, int32) ([]promo.PendingReward, error) {
	return nil, nil
}
func (s *stubPromoRepo) CountGranted(context.Context, int64) (int64, error) { return 0, nil }
func (s *stubPromoRepo) Grant(context.Context, int64, int64, int64) (bool, error) {
	return false, nil
}
func (s *stubPromoRepo) MarkDelivered(context.Context, int64) (bool, error) { return false, nil }
func (s *stubPromoRepo) UndeliveredRewards(context.Context, int32) ([]promo.EarnedReward, error) {
	return nil, nil
}

// promoApp mounts the preview route with a caller already attached, so the test exercises
// the handler rather than the auth middleware.
func promoApp(t *testing.T, repo *stubPromoRepo, callerID int64) *fiber.App {
	t.Helper()
	h := newPromoHandlers(promo.New(repo, promo.Config{SiteURL: "https://example.test"}))
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/me/promo/preview", func(c *fiber.Ctx) error {
		if callerID > 0 {
			c.Locals(auth.LocalsUserID, callerID)
		}
		return c.Next()
	}, h.PreviewCode)
	return app
}

// postPreview sends one preview request and returns its status, closing the body before it
// returns so no test has to remember to.
func postPreview(t *testing.T, app *fiber.App, body string) int {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		"/me/promo/preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

func TestPreviewRefusesAnAnonymousCaller(t *testing.T) {
	repo := &stubPromoRepo{usable: map[string]int16{"ZZTEST90": 90}, redeemed: map[int64]bool{}}
	status := postPreview(t, promoApp(t, repo, 0), `{"code":"ZZTEST90"}`)

	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
	if repo.previews != 0 {
		t.Fatalf("previews = %d, want 0 — nothing is read for a caller we do not know",
			repo.previews)
	}
}

func TestPreviewAnswersWithThePercentage(t *testing.T) {
	repo := &stubPromoRepo{usable: map[string]int16{"ZZTEST90": 90}, redeemed: map[int64]bool{}}
	status := postPreview(t, promoApp(t, repo, 7), `{"code":"zztest90"}`)

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
}

func TestPreviewConsumesNoSeat(t *testing.T) {
	repo := &stubPromoRepo{usable: map[string]int16{"ZZTEST90": 90}, redeemed: map[int64]bool{}}
	app := promoApp(t, repo, 7)

	_ = postPreview(t, app, `{"code":"ZZTEST90"}`)
	_ = postPreview(t, app, `{"code":"ZZTEST90"}`)

	if repo.redeems != 0 {
		t.Fatalf("redeems = %d, want 0 — previewing is what somebody does while typing, and "+
			"it must not spend a seat of a capped launch offer", repo.redeems)
	}
}

func TestPreviewGivesOneAnswerToEveryRefusalAboutTheCode(t *testing.T) {
	repo := &stubPromoRepo{usable: map[string]int16{"ZZTEST90": 90}, redeemed: map[int64]bool{}}
	app := promoApp(t, repo, 7)

	unknown := postPreview(t, app, `{"code":"ZZNONE00"}`)
	malformed := postPreview(t, app, `{"code":"!!"}`)

	if unknown != http.StatusNotFound || malformed != http.StatusNotFound {
		t.Fatalf("statuses = %d and %d, want 404 for both — telling 'no such code' apart "+
			"from 'not eligible' turns this route into an oracle for guessing codes",
			unknown, malformed)
	}
}

func TestPreviewSaysWhenTheCallerHasAlreadyRedeemed(t *testing.T) {
	repo := &stubPromoRepo{usable: map[string]int16{"ZZTEST90": 90}, redeemed: map[int64]bool{7: true}}
	status := postPreview(t, promoApp(t, repo, 7), `{"code":"ZZTEST90"}`)

	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409 — this one is about the caller, discloses nothing "+
			"about any code, and is the answer that actually helps them", status)
	}
}
