//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
)

type replyBody struct {
	Data struct {
		Recipient string `json:"recipient"`
		Subject   string `json:"subject"`
		Body      string `json:"body"`
	} `json:"data"`
	Error string `json:"error"`
}

// postReplyDraft mounts the route over a model answering reply/err, with one draft a day.
func postReplyDraft(t *testing.T, fx recallFixture, model *recallModel, userID, emailID int64) (int, replyBody, *plan.Store) {
	t.Helper()
	queries := db.New(fx.pool)
	makePro(t, fx.pool, fx.userID)
	plans := plan.NewStore(queries, fx.pool, plan.DefaultConfig().WithProCeiling(plan.FeatureMailReply, 1).Enforcing())
	client := llm.NewWithModel(model)
	h := newInboxHandlers(queries, fx.pool, nil, nil, "", false, "inbox.freehire.test", plans).
		withRecall(nil, llmBinding{client: client}, nil)

	iss := auth.NewIssuer("test-secret-that-is-long-enough-0001", time.Hour)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/me/emails/:id/reply-draft",
		auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries}), markPaidAI(plan.FeatureMailReply),
		newContract(contractHandlers{inboxHandlers: h}))

	cookie, _ := iss.Issue(userID, testTokenVersion)
	r := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/me/emails/"+strconv.FormatInt(emailID, 10)+"/reply-draft", nil)
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	resp, err := app.Test(r, -1)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	var body replyBody
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body, plans
}

func used(t *testing.T, plans *plan.Store, userID int64) int {
	t.Helper()
	st, err := plans.Standing(context.Background(), userID, plan.FeatureMailReply)
	if err != nil {
		t.Fatalf("standing: %v", err)
	}
	return st.Used
}

func TestDraftEmailReply_ReturnsADraftAndChargesOnce(t *testing.T) {
	fx := seedRecallFixture(t, startPostgres(t))
	if _, err := fx.pool.Exec(context.Background(),
		`UPDATE emails SET from_addr = 'anna@derq.io' WHERE id = $1`, fx.unlinked); err != nil {
		t.Fatal(err)
	}
	model := &recallModel{reply: `{"body":"Hi Anna,\n\nThank you.\n\n[Your name]"}`}
	status, body, plans := postReplyDraft(t, fx, model, fx.userID, fx.unlinked)
	if status != fiber.StatusOK {
		t.Fatalf("status %d, body %+v", status, body)
	}
	if body.Data.Subject != "Re: Thanks for applying to Derq" || body.Data.Recipient != "anna@derq.io" ||
		body.Data.Body == "" {
		t.Errorf("draft = %+v", body.Data)
	}
	if n := used(t, plans, fx.userID); n != 1 {
		t.Errorf("used = %d, want 1", n)
	}
}

func TestDraftEmailReply_ModelFailureReturnsTheCharge(t *testing.T) {
	fx := seedRecallFixture(t, startPostgres(t))
	model := &recallModel{err: errors.New("provider says: the prompt was ...")}
	status, body, plans := postReplyDraft(t, fx, model, fx.userID, fx.unlinked)
	if status != fiber.StatusBadGateway {
		t.Fatalf("status %d, want 502 (body %+v)", status, body)
	}
	if n := used(t, plans, fx.userID); n != 0 {
		t.Errorf("used = %d, want the charge returned", n)
	}
}

func TestDraftEmailReply_RefusesASpentAllowance(t *testing.T) {
	fx := seedRecallFixture(t, startPostgres(t))
	model := &recallModel{reply: `{"body":"Hi"}`}
	if status, _, _ := postReplyDraft(t, fx, model, fx.userID, fx.unlinked); status != fiber.StatusOK {
		t.Fatalf("first draft status %d", status)
	}
	status, body, _ := postReplyDraft(t, fx, model, fx.userID, fx.unlinked)
	if status != fiber.StatusPaymentRequired || body.Error == "" {
		t.Fatalf("second draft status %d body %+v, want 402 with a sentence", status, body)
	}
	if model.calls != 1 {
		t.Errorf("model calls = %d, want the refusal before the model", model.calls)
	}
}

func TestDraftEmailReply_RefusesSentAndForeignMail(t *testing.T) {
	fx := seedRecallFixture(t, startPostgres(t))
	ctx := context.Background()
	if _, err := fx.pool.Exec(ctx, `UPDATE emails SET direction = 'outgoing' WHERE id = $1`, fx.linked); err != nil {
		t.Fatal(err)
	}
	var stranger int64
	if err := fx.pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('reply-stranger@example.test') RETURNING id`).Scan(&stranger); err != nil {
		t.Fatal(err)
	}
	model := &recallModel{reply: `{"body":"Hi"}`}
	if status, _, _ := postReplyDraft(t, fx, model, fx.userID, fx.linked); status != fiber.StatusConflict {
		t.Errorf("sent message: status %d, want 409", status)
	}
	if status, _, _ := postReplyDraft(t, fx, model, stranger, fx.unlinked); status != fiber.StatusNotFound {
		t.Errorf("another user's message: status %d, want 404", status)
	}
	if model.calls != 0 {
		t.Errorf("model calls = %d, want none", model.calls)
	}
}
