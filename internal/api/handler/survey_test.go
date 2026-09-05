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

	"github.com/strelov1/freehire/internal/candidate/survey"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// fakeSurveyRepo is a survey.Repository returning canned records and recording the upsert
// it is handed, so the handler tests run without a database. The DB-backed contract is
// covered by the store's own tests.
type fakeSurveyRepo struct {
	getRet survey.Responses
	getErr error

	upserted     survey.Responses
	upsertCalled bool
	upsertErr    error
}

func (f *fakeSurveyRepo) Get(context.Context, int64) (survey.Responses, error) {
	return f.getRet, f.getErr
}

func (f *fakeSurveyRepo) Upsert(_ context.Context, _ int64, a survey.Responses) (survey.Responses, error) {
	f.upserted = a
	f.upsertCalled = true
	return a, f.upsertErr
}

// fakeOnboardingStore records whether the completion marker was written.
type fakeOnboardingStore struct {
	markedUserID int64
	markCalled   bool
	markErr      error
}

func (f *fakeOnboardingStore) MarkOnboardingComplete(_ context.Context, userID int64) error {
	f.markedUserID = userID
	f.markCalled = true
	return f.markErr
}

func surveyApp(t *testing.T, repo *fakeSurveyRepo, onb *fakeOnboardingStore) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &surveyHandlers{store: survey.New(repo), onboarding: onb}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	g := auth.RequireAuth(iss, testVersions)
	app.Get("/me/survey", g, h.GetSurvey)
	app.Put("/me/survey", g, h.PutSurvey)
	app.Post("/me/onboarding/complete", g, h.CompleteOnboarding)
	return app, token
}

func doSurvey(t *testing.T, app *fiber.App, method, path, body, token string) *http.Response {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequestWithContext(context.Background(), method, path, nil)
	} else {
		r = httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(r)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

func TestGetSurveyOnAnUnansweredAccountReturnsAnEmptyRecord(t *testing.T) {
	// Not a 404 and not {"data": null}: the wizard reads this to decide which steps to
	// skip, and "no answers" must arrive as an object it can inspect field by field.
	app, token := surveyApp(t, &fakeSurveyRepo{getErr: survey.ErrNotFound}, &fakeOnboardingStore{})

	resp := doSurvey(t, app, http.MethodGet, "/me/survey", "", token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data == nil {
		t.Fatalf("data = null, want an object with every field unstated")
	}
	if len(body.Data) != 0 {
		t.Errorf("data = %v, want no stated fields", body.Data)
	}
}

func TestGetSurveyRequiresAuth(t *testing.T) {
	app, _ := surveyApp(t, &fakeSurveyRepo{}, &fakeOnboardingStore{})

	resp := doSurvey(t, app, http.MethodGet, "/me/survey", "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestPutSurveyStoresTheStatedFields(t *testing.T) {
	repo := &fakeSurveyRepo{}
	app, token := surveyApp(t, repo, &fakeOnboardingStore{})

	resp := doSurvey(t, app, http.MethodPut, "/me/survey", `{"job_search_stage":"searching"}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if !repo.upsertCalled {
		t.Fatal("Upsert was not called")
	}
	if repo.upserted.JobSearchStage == nil || *repo.upserted.JobSearchStage != "searching" {
		t.Errorf("stored stage = %v, want searching", repo.upserted.JobSearchStage)
	}
}

func TestPutSurveyRejectsAnOutOfVocabularyValueWith400(t *testing.T) {
	repo := &fakeSurveyRepo{}
	app, token := surveyApp(t, repo, &fakeOnboardingStore{})

	resp := doSurvey(t, app, http.MethodPut, "/me/survey", `{"biggest_challenge":"vibes"}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if repo.upsertCalled {
		t.Error("Upsert was called despite invalid input")
	}
}

func TestPutSurveyRejectsAMalformedBody(t *testing.T) {
	app, token := surveyApp(t, &fakeSurveyRepo{}, &fakeOnboardingStore{})

	resp := doSurvey(t, app, http.MethodPut, "/me/survey", `{`, token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestCompleteOnboardingMarksTheCaller(t *testing.T) {
	onb := &fakeOnboardingStore{}
	app, token := surveyApp(t, &fakeSurveyRepo{}, onb)

	resp := doSurvey(t, app, http.MethodPost, "/me/onboarding/complete", "", token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if !onb.markCalled || onb.markedUserID != 1 {
		t.Fatalf("marked user %d (called=%v), want the authenticated caller", onb.markedUserID, onb.markCalled)
	}
}

func TestCompleteOnboardingIsIdempotent(t *testing.T) {
	// The marker query is guarded on IS NULL, so a repeat call affects no rows. That is a
	// success, not a conflict — a double-clicked finish button must not read as an error.
	onb := &fakeOnboardingStore{}
	app, token := surveyApp(t, &fakeSurveyRepo{}, onb)

	for i := range 2 {
		resp := doSurvey(t, app, http.MethodPost, "/me/onboarding/complete", "", token)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("call %d: status = %d, want 200", i+1, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestCompleteOnboardingRequiresAuth(t *testing.T) {
	onb := &fakeOnboardingStore{}
	app, _ := surveyApp(t, &fakeSurveyRepo{}, onb)

	resp := doSurvey(t, app, http.MethodPost, "/me/onboarding/complete", "", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if onb.markCalled {
		t.Error("an unauthenticated request reached the store")
	}
}

func TestCompleteOnboardingSurfacesAStoreFailure(t *testing.T) {
	onb := &fakeOnboardingStore{markErr: errors.New("connection refused")}
	app, token := surveyApp(t, &fakeSurveyRepo{}, onb)

	resp := doSurvey(t, app, http.MethodPost, "/me/onboarding/complete", "", token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
}
