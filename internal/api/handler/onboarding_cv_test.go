package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/pii"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// onboardingCV is the CV every test here reads: a headline classify resolves, a skills
// section skilltag resolves, a location the residence dictionary resolves, and contact
// details that must never come back.
const onboardingCV = `Jane Doe
Senior Backend Engineer
jane@example.com · +1 555 0100 · linkedin.com/in/janedoe
Rua Bocaiúva 2468, Florianópolis, Brazil

Skills
Go, PostgreSQL, Kubernetes, Docker

Experience
Senior Backend Engineer, Finbridge, 2021 - Present
Backend Engineer, Sber, 2017 - 2021
Engineer, Informa, 2014 - 2017`

// onboardingModelJSON is what the model returns for onboardingCV.
const onboardingModelJSON = `{"headline":"Senior software engineer, high-availability backends",` +
	`"location":"Florianópolis, Brazil","total_years":11,` +
	`"email":"jane@example.com","phone":"+1 555 0100","links":["linkedin.com/in/janedoe"],` +
	`"experience":[` +
	`{"title":"Senior Backend Engineer","company":"Finbridge","start":"2021","current":true},` +
	`{"title":"Backend Engineer","company":"Sber","start":"2017","end":"2021"},` +
	`{"title":"Engineer","company":"Sber","start":"2016","end":"2017"},` +
	`{"title":"Engineer","company":"Informa","start":"2014","end":"2016"},` +
	`{"title":"Intern","company":"RingCentral","start":"2013","end":"2014"},` +
	`{"title":"Intern","company":"Acme","start":"2012","end":"2013"}]}`

// contactDetector reports the CV's name and residence the way the PII service does: the
// extractor takes both from these spans, never from the model.
type contactDetector struct{}

func (contactDetector) Detect(_ context.Context, text string) ([]pii.Span, error) {
	var spans []pii.Span
	for value, kind := range map[string]string{"Jane Doe": pii.KindName, "Rua Bocaiúva 2468, Florianópolis, Brazil": pii.KindAddress} {
		if i := strings.Index(text, value); i >= 0 {
			spans = append(spans, pii.Span{Start: i, End: i + len(value), Kind: kind})
		}
	}
	return spans, nil
}

func onboardingApp(h *resumeHandlers, userID int64) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Use(func(c *fiber.Ctx) error {
		if userID != 0 {
			c.Locals(auth.LocalsUserID, userID)
		}
		return c.Next()
	})
	app.Post("/onboarding/cv", h.ParseOnboardingCV)
	return app
}

func onboardingHandlersWith(t *testing.T, client *llm.Client, keys *readCountingKeys) *resumeHandlers {
	t.Helper()
	return &resumeHandlers{
		structuredExtractor: resumeextract.NewExtractor(client, contactDetector{}),
		llm:                 intentBinding(t, client, keys),
	}
}

type onboardingCVAnswer struct {
	Data struct {
		StructuredAvailable bool            `json:"structured_available"`
		FirstName           string          `json:"first_name"`
		Headline            string          `json:"headline"`
		Country             string          `json:"country"`
		TotalYears          int             `json:"total_years"`
		Companies           []string        `json:"companies"`
		Skills              []string        `json:"skills"`
		Categories          []string        `json:"categories"`
		Seniority           string          `json:"seniority"`
		Report              json.RawMessage `json:"report"`
	} `json:"data"`
}

func onboardingRequest(body string) *http.Request {
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/onboarding/cv", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func postOnboardingCV(t *testing.T, app *fiber.App, cv string) (int, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"text": cv})
	resp, err := app.Test(onboardingRequest(string(body)), -1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func TestParseOnboardingCV_ReadsTheCVForAGuest(t *testing.T) {
	model := &capturingModel{}
	keys := newReadCountingKeys()
	h := onboardingHandlersWith(t, model.client(t, onboardingModelJSON), keys)

	status, raw := postOnboardingCV(t, onboardingApp(h, 0), onboardingCV)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, raw)
	}
	var out onboardingCVAnswer
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decode: %v: %s", err, raw)
	}
	d := out.Data
	if !d.StructuredAvailable {
		t.Fatalf("structured_available = false, want true: %s", raw)
	}
	if d.FirstName != "Jane" {
		t.Errorf("first_name = %q, want %q (the first name only)", d.FirstName, "Jane")
	}
	if d.Headline == "" || d.TotalYears != 11 {
		t.Errorf("headline/years = %q/%d, want the model's", d.Headline, d.TotalYears)
	}
	if d.Country != "br" {
		t.Errorf("country = %q, want br (resolved from the stated location)", d.Country)
	}
	wantCompanies := []string{"Finbridge", "Sber", "Informa", "RingCentral"}
	if strings.Join(d.Companies, ",") != strings.Join(wantCompanies, ",") {
		t.Errorf("companies = %v, want %v (distinct, most recent first, at most four)", d.Companies, wantCompanies)
	}
	if len(d.Skills) == 0 || len(d.Categories) == 0 || d.Seniority == "" {
		t.Errorf("skills/categories/seniority = %v/%v/%q, want the dictionary profile", d.Skills, d.Categories, d.Seniority)
	}
	if len(d.Report) == 0 || string(d.Report) == "null" {
		t.Errorf("report missing: %s", raw)
	}
	// The residence is never sent as text, only as the dictionary's country code: an
	// address span can be a street address in any shape.
	for _, leaked := range []string{"jane@example.com", "555 0100", "linkedin.com/in/janedoe", "Doe", "Bocaiúva", `"location"`} {
		if strings.Contains(raw, leaked) {
			t.Errorf("answer leaks %q: %s", leaked, raw)
		}
	}

	auths, features := model.calls()
	if len(auths) == 0 {
		t.Fatal("the model was never called")
	}
	for i := range auths {
		if auths[i] != "Bearer sk-service" || features[i] != tagCVOnboard {
			t.Errorf("call %d = %q/%q, want the service credential tagged %q", i, auths[i], features[i], tagCVOnboard)
		}
	}
	if n := keys.totalReads(); n != 0 {
		t.Fatalf("resolver read %d credentials for a guest, want 0", n)
	}
}

func TestParseOnboardingCV_SignedInSpendsAsTheCaller(t *testing.T) {
	model := &capturingModel{}
	keys := newReadCountingKeys()
	keys.stored[7], keys.storedIDs[7] = "sk-user-7", "vk-user-7"
	h := onboardingHandlersWith(t, model.client(t, onboardingModelJSON), keys)

	status, raw := postOnboardingCV(t, onboardingApp(h, 7), onboardingCV)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, raw)
	}
	auths, features := model.calls()
	if len(auths) == 0 || auths[0] != "Bearer sk-user-7" || features[0] != tagCVOnboard {
		t.Fatalf("calls = %v / %v, want the caller's own credential tagged %q", auths, features, tagCVOnboard)
	}
}

func TestParseOnboardingCV_ModelFailureKeepsTheDictionaryProfile(t *testing.T) {
	model := &capturingModel{}
	keys := newReadCountingKeys()
	// Not JSON: the extractor fails to decode it.
	h := onboardingHandlersWith(t, model.client(t, "not json at all"), keys)

	status, raw := postOnboardingCV(t, onboardingApp(h, 0), onboardingCV)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 (a degraded answer, not an error): %s", status, raw)
	}
	var out onboardingCVAnswer
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	d := out.Data
	if d.StructuredAvailable {
		t.Errorf("structured_available = true, want false when the model failed")
	}
	if d.FirstName != "" || d.Headline != "" || len(d.Companies) != 0 {
		t.Errorf("structured fields = %q/%q/%v, want empty rather than guessed", d.FirstName, d.Headline, d.Companies)
	}
	if len(d.Skills) == 0 || len(d.Categories) == 0 || len(d.Report) == 0 {
		t.Errorf("dictionary profile or report missing: %s", raw)
	}
}

func TestParseOnboardingCV_NoExtractorStillAnswers(t *testing.T) {
	h := &resumeHandlers{}
	status, raw := postOnboardingCV(t, onboardingApp(h, 0), onboardingCV)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, raw)
	}
	if !strings.Contains(raw, `"structured_available":false`) {
		t.Errorf("want structured_available:false with no extractor: %s", raw)
	}
}

func TestParseOnboardingCV_RefusesAnEmptyCV(t *testing.T) {
	h := &resumeHandlers{}
	status, _ := postOnboardingCV(t, onboardingApp(h, 0), "   ")
	if status != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

func TestParseOnboardingCV_GuestEncodedBodyRefused(t *testing.T) {
	h := &resumeHandlers{}
	app := onboardingApp(h, 0)
	req := onboardingRequest(`{"text":"x"}`)
	req.Header.Set("Content-Encoding", "gzip")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", resp.StatusCode)
	}
}
