package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/screeninganswers"
)

// fakeScreeningAnswersRepo is a screeninganswers.Repository that returns canned
// records/errors and records the upsert it is handed, so the handler tests run without a
// database. The DB-backed contract is covered by the store's own tests.
type fakeScreeningAnswersRepo struct {
	getRet screeninganswers.Answers
	getErr error

	upserted     screeninganswers.Answers
	upsertCalled bool
	upsertRet    screeninganswers.Answers
	upsertErr    error
}

func (f *fakeScreeningAnswersRepo) Get(context.Context, int64) (screeninganswers.Answers, error) {
	return f.getRet, f.getErr
}

func (f *fakeScreeningAnswersRepo) Upsert(_ context.Context, _ int64, a screeninganswers.Answers) (screeninganswers.Answers, error) {
	f.upserted = a
	f.upsertCalled = true
	return f.upsertRet, f.upsertErr
}

// screeningAnswersApp mounts the singleton screening-answers endpoints behind auth on a
// handler backed by the given in-memory fake repo.
func screeningAnswersApp(t *testing.T, repo *fakeScreeningAnswersRepo) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &screeningAnswersHandlers{store: screeninganswers.New(repo)}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	g := auth.RequireAuth(iss, testVersions)
	app.Get("/me/screening-answers", g, h.GetScreeningAnswers)
	app.Put("/me/screening-answers", g, h.PutScreeningAnswers)
	return app, token
}

func doScreeningAnswers(t *testing.T, app *fiber.App, method, body, token string) *http.Response {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequestWithContext(context.Background(), method, "/me/screening-answers", nil)
	} else {
		r = httptest.NewRequestWithContext(context.Background(), method, "/me/screening-answers", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(r)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	return resp
}

func TestGetScreeningAnswers_Unauthenticated(t *testing.T) {
	app, _ := screeningAnswersApp(t, &fakeScreeningAnswersRepo{})
	resp := doScreeningAnswers(t, app, fiber.MethodGet, "", "")
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestGetScreeningAnswers_NoRecordYetReturnsNullData(t *testing.T) {
	app, token := screeningAnswersApp(t, &fakeScreeningAnswersRepo{getErr: screeninganswers.ErrNotFound})
	resp := doScreeningAnswers(t, app, fiber.MethodGet, "", token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got struct {
		Data *screeninganswers.Answers `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Data != nil {
		t.Errorf("data = %+v, want null", got.Data)
	}
}

func TestGetScreeningAnswers_ReturnsStoredRecord(t *testing.T) {
	days := 30
	ret := screeninganswers.Answers{NoticePeriodDays: &days}
	app, token := screeningAnswersApp(t, &fakeScreeningAnswersRepo{getRet: ret})
	resp := doScreeningAnswers(t, app, fiber.MethodGet, "", token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got struct {
		Data screeninganswers.Answers `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Data.NoticePeriodDays == nil || *got.Data.NoticePeriodDays != 30 {
		t.Errorf("NoticePeriodDays = %v, want 30", got.Data.NoticePeriodDays)
	}
}

func TestPutScreeningAnswers_Unauthenticated(t *testing.T) {
	app, _ := screeningAnswersApp(t, &fakeScreeningAnswersRepo{})
	resp := doScreeningAnswers(t, app, fiber.MethodPut, `{"willing_to_relocate":true}`, "")
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestPutScreeningAnswers_PartialUpdateMergesAndReturnsRecord(t *testing.T) {
	repo := &fakeScreeningAnswersRepo{
		getErr:    screeninganswers.ErrNotFound,
		upsertRet: screeninganswers.Answers{WillingToRelocate: boolPtr(true)},
	}
	app, token := screeningAnswersApp(t, repo)
	resp := doScreeningAnswers(t, app, fiber.MethodPut, `{"willing_to_relocate":true}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if !repo.upsertCalled {
		t.Fatal("repo.Upsert was not called")
	}
	if repo.upserted.WillingToRelocate == nil || *repo.upserted.WillingToRelocate != true {
		t.Errorf("upserted WillingToRelocate = %v, want true", repo.upserted.WillingToRelocate)
	}
	var got struct {
		Data screeninganswers.Answers `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Data.WillingToRelocate == nil || *got.Data.WillingToRelocate != true {
		t.Errorf("response WillingToRelocate = %v, want true", got.Data.WillingToRelocate)
	}
}

func TestPutScreeningAnswers_InvalidFieldReturns400AndSkipsUpsert(t *testing.T) {
	repo := &fakeScreeningAnswersRepo{getErr: screeninganswers.ErrNotFound}
	app, token := screeningAnswersApp(t, repo)
	resp := doScreeningAnswers(t, app, fiber.MethodPut, `{"desired_salary_currency":"usd!"}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	if repo.upsertCalled {
		t.Error("repo.Upsert should not be called on invalid input")
	}
}

func TestPutScreeningAnswers_RepositoryFailureReturns500NotItsRawMessage(t *testing.T) {
	repo := &fakeScreeningAnswersRepo{getErr: errors.New("connection refused")}
	app, token := screeningAnswersApp(t, repo)
	resp := doScreeningAnswers(t, app, fiber.MethodPut, `{"willing_to_relocate":true}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	var got struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if strings.Contains(got.Error, "connection refused") {
		t.Errorf("response body leaked the internal error: %q", got.Error)
	}
}

func boolPtr(b bool) *bool { return &b }
