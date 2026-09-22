package enrich

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wawan93/gojev"
)

// TestToChoiceMap tests converting vocabulary slices to choice maps with identity mappings and null fallback.
func TestToChoiceMap(t *testing.T) {
	values := []string{"foo", "bar"}
	m := toChoiceMap(values)

	if m["foo"] != "foo" || m["bar"] != "bar" {
		t.Errorf("expected values mapped to themselves, got %+v", m)
	}
	if m["null"] != "Not stated in the posting" {
		t.Errorf("expected null key with fallback text, got %v", m["null"])
	}
}

// TestToChoiceMapWithGloss tests converting vocabulary values with explanatory gloss definitions.
func TestToChoiceMapWithGloss(t *testing.T) {
	values := []string{"product", "agency"}
	gloss := map[string]string{"product": "Builds own product"}
	m := toChoiceMapWithGloss(values, gloss)

	if m["product"] != "Builds own product" {
		t.Errorf("expected gloss mapping, got %v", m["product"])
	}
	if m["agency"] != "agency" {
		t.Errorf("expected value fallback, got %v", m["agency"])
	}
	if m["null"] != "Not stated in the posting" {
		t.Errorf("expected null key with fallback text, got %v", m["null"])
	}
}

// TestJevProvider_Enrich_Success tests successful job enrichment extraction from Jev SystemOne response.
func TestJevProvider_Enrich_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"model": "jev-1",
			"answers": map[string]any{
				"relocation": map[string]any{
					"type":   "choice",
					"choice": "supported",
				},
				"salary_period": map[string]any{
					"type":   "choice",
					"choice": "year",
				},
				"company_size": map[string]any{
					"type":   "choice",
					"choice": "51-200",
				},
				"company_type": map[string]any{
					"type":   "choice",
					"choice": "product",
				},
				"visa_sponsorship": map[string]any{
					"type": "noul",
					"noul": 1.0,
				},
				"region": map[string]any{
					"type":   "choice",
					"choice": "eu",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client, err := gojev.NewClient("test-key", gojev.WithBaseURL(ts.URL))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	p := NewJevProvider(client)
	job := JobInput{
		Title:           "Senior Go Engineer",
		Company:         "Acme",
		Location:        "Berlin",
		Description:     "Job description",
		CompanyTypeHint: "product",
		Remote:          true,
		GeoPinned:       false,
	}

	got, err := p.Enrich(context.Background(), job)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	if got.Relocation != "supported" {
		t.Errorf("relocation = %q, want supported", got.Relocation)
	}
	if got.SalaryPeriod != "year" {
		t.Errorf("salary_period = %q, want year", got.SalaryPeriod)
	}
	if got.CompanySize != "51-200" {
		t.Errorf("company_size = %q, want 51-200", got.CompanySize)
	}
	if got.CompanyType != "product" {
		t.Errorf("company_type = %q, want product", got.CompanyType)
	}
	if got.VisaSponsorship == nil || *got.VisaSponsorship != true {
		t.Errorf("visa_sponsorship = %v, want true", got.VisaSponsorship)
	}
	if len(got.Regions) != 1 || got.Regions[0] != "eu" {
		t.Errorf("regions = %v, want [eu]", got.Regions)
	}
}

// TestJevProvider_Enrich_NegativeVisaAndNullChoices tests handling of null choices and negative noul values.
func TestJevProvider_Enrich_NegativeVisaAndNullChoices(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"model": "jev-1",
			"answers": map[string]any{
				"relocation": map[string]any{
					"type":   "choice",
					"choice": "null",
				},
				"salary_period": map[string]any{
					"type":   "choice",
					"choice": "none",
				},
				"visa_sponsorship": map[string]any{
					"type": "noul",
					"noul": -0.8,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client, err := gojev.NewClient("test-key", gojev.WithBaseURL(ts.URL))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	p := NewJevProvider(client)
	job := JobInput{
		Title:       "Junior Developer",
		Company:     "Corp",
		GeoPinned:   true,
		Description: "Junior dev role",
	}

	got, err := p.Enrich(context.Background(), job)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	if got.Relocation != "" {
		t.Errorf("relocation = %q, want empty", got.Relocation)
	}
	if got.SalaryPeriod != "" {
		t.Errorf("salary_period = %q, want empty", got.SalaryPeriod)
	}
	if got.VisaSponsorship == nil || *got.VisaSponsorship != false {
		t.Errorf("visa_sponsorship = %v, want false", got.VisaSponsorship)
	}
	if len(got.Regions) != 0 {
		t.Errorf("regions = %v, want empty when GeoPinned", got.Regions)
	}
}

// TestJevProvider_Enrich_ServerError tests error propagation when the Jev upstream server returns an error.
func TestJevProvider_Enrich_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer ts.Close()

	client, err := gojev.NewClient("test-key", gojev.WithBaseURL(ts.URL))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	p := NewJevProvider(client)
	_, err = p.Enrich(context.Background(), JobInput{Title: "Engineer"})
	if err == nil {
		t.Fatal("expected error on server failure, got nil")
	}
}
