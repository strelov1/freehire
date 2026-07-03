package atscheck

import (
	"reflect"
	"strings"
	"testing"
)

func sectionItem(t *testing.T, r Report, substr string) (LineItem, bool) {
	t.Helper()
	for _, it := range catByID(t, r, "section_completeness").Items {
		if strings.Contains(strings.ToLower(it.Text), substr) {
			return it, true
		}
	}
	return LineItem{}, false
}

func TestScore_SummaryKeywordDensity_Dense(t *testing.T) {
	cv := "Jane Roe\njane@example.com  +1 415 555 0134\n\nSummary\nBackend engineer. Core stack: Golang, Kafka, Kubernetes, PostgreSQL.\n\nExperience\n- Built services\n\nSkills\nGolang, Kafka"
	r := Score(cv, []string{"go", "kafka"}, nil)
	it, ok := sectionItem(t, r, "keyword")
	if !ok {
		t.Fatalf("no summary keyword-density item in %+v", catByID(t, r, "section_completeness").Items)
	}
	if it.Status != StatusPass {
		t.Errorf("density = %s, want pass for a keyword-dense summary", it.Status)
	}
}

func TestScore_SummaryKeywordDensity_Generic(t *testing.T) {
	cv := "Jane Roe\njane@example.com  +1 415 555 0134\n\nSummary\nExperienced engineer passionate about building great products.\n\nExperience\n- Built services\n\nSkills\nGolang"
	r := Score(cv, []string{"go"}, nil)
	it, ok := sectionItem(t, r, "keyword")
	if !ok {
		t.Fatal("no summary keyword-density item")
	}
	if it.Status == StatusPass {
		t.Errorf("density = pass, want recoverable for a generic summary")
	}
}

func TestScore_CategoryMaximaSumTo100(t *testing.T) {
	r := Score(cleanCV, cleanSkills, []string{"go", "kafka"})
	sum := 0
	for _, c := range r.Categories {
		sum += c.Max
	}
	if sum != 100 {
		t.Errorf("Σ category max = %d, want 100", sum)
	}
}

// cleanCV is a realistic plain-text CV that should pass every deterministic check
// (kept > the length floor so the length check passes honestly).
const cleanCV = `Ilya Ivanov
ilya@example.com  +1 415 555 0134  San Francisco, CA

Summary
Senior backend engineer with eight years building high-throughput distributed
systems and data platforms. Comfortable owning services end to end, from design
and implementation through on-call and cost optimization. Focused on reliability,
observability, and keeping large systems affordable to run at scale over time.

Experience
Senior Backend Engineer, Acme (2021 - 2026)
- Built distributed services in Go handling 20,000 requests per second
- Led the migration to Kubernetes across 40 services, cutting infra cost 30%
- Designed a Kafka-based event pipeline processing 2 billion events per day
- Mentored four engineers and ran the backend on-call rotation

Backend Engineer, Globex (2018 - 2021)
- Shipped a PostgreSQL-backed billing service used by 3 million customers
- Introduced Terraform and CI/CD, reducing deploy time from hours to minutes
- Cut p99 latency 45% by adding Redis caching and query tuning

Education
BSc Computer Science, MIT (2014 - 2018)

Skills
Go, Kafka, PostgreSQL, Docker, Kubernetes, AWS, Terraform, Redis, gRPC, Python`

var cleanSkills = []string{"go", "kafka", "postgresql", "docker", "kubernetes"}

func catByID(t *testing.T, r Report, id string) ScoreCategory {
	t.Helper()
	for _, c := range r.Categories {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no category %q in %+v", id, r.Categories)
	return ScoreCategory{}
}

func sumScores(r Report) int {
	sum := 0
	for _, c := range r.Categories {
		sum += c.Score
	}
	return sum
}

func TestScore_ScannedCVFailsMachineReadable(t *testing.T) {
	r := Score("   ", nil, nil)
	fmtCat := catByID(t, r, "format_compliance")
	var found bool
	for _, it := range fmtCat.Items {
		if it.Status == StatusFail {
			found = true
		}
	}
	if !found {
		t.Errorf("format_compliance has no fail item for a scan: %+v", fmtCat.Items)
	}
	if r.Overall > 30 {
		t.Errorf("Overall = %d, want low for a scan", r.Overall)
	}
}

func TestScore_CleanCVMaxesDeterministicCategories(t *testing.T) {
	// No role → keyword category scores 0; the four deterministic categories should
	// each max out: 20 + 15 + 15 + 10 = 60.
	r := Score(cleanCV, cleanSkills, nil)
	if got := catByID(t, r, "format_compliance").Score; got != 20 {
		t.Errorf("format score = %d, want 20", got)
	}
	if got := catByID(t, r, "section_completeness").Score; got != 15 {
		t.Errorf("section score = %d, want 15", got)
	}
	if got := catByID(t, r, "content_quality").Score; got != 15 {
		t.Errorf("content score = %d, want 15", got)
	}
	if got := catByID(t, r, "length_density").Score; got != 10 {
		t.Errorf("length score = %d, want 10", got)
	}
	if r.Overall != 60 {
		t.Errorf("Overall = %d, want 60", r.Overall)
	}
	if r.Potential != 100 {
		t.Errorf("Potential = %d, want 100 (keyword 40 recoverable)", r.Potential)
	}
}

func TestScore_KeywordStrengthAndRecommended(t *testing.T) {
	// CV has go + kafka; role wants go, kubernetes, kafka → 2/3 × 40 = 27.
	r := Score(cleanCV, []string{"go", "kafka"}, []string{"go", "kubernetes", "kafka"})
	if got := catByID(t, r, "keyword_strength").Score; got != 27 {
		t.Errorf("keyword score = %d, want 27", got)
	}
	if want := []string{"go", "kafka"}; !reflect.DeepEqual(r.StrongKeywords, want) {
		t.Errorf("StrongKeywords = %v, want %v", r.StrongKeywords, want)
	}
	if want := []string{"kubernetes"}; !reflect.DeepEqual(r.RecommendedKeywords, want) {
		t.Errorf("RecommendedKeywords = %v, want %v", r.RecommendedKeywords, want)
	}
}

func TestScore_KeywordFullMatchMaxesCategory(t *testing.T) {
	r := Score(cleanCV, []string{"go", "kafka"}, []string{"go", "kafka"})
	if got := catByID(t, r, "keyword_strength").Score; got != 40 {
		t.Errorf("keyword score = %d, want 40 for a full match", got)
	}
}

func TestScore_OverallIsSumOfCategories(t *testing.T) {
	r := Score(cleanCV, []string{"go", "kafka"}, []string{"go", "kubernetes", "kafka"})
	if r.Overall != sumScores(r) {
		t.Errorf("Overall = %d, want sum of category scores %d", r.Overall, sumScores(r))
	}
}

func TestScore_PotentialAtLeastOverallCappedAt100(t *testing.T) {
	r := Score(cleanCV, []string{"go"}, []string{"go", "kubernetes", "kafka", "grpc"})
	if r.Potential < r.Overall {
		t.Errorf("Potential = %d, want ≥ Overall %d", r.Potential, r.Overall)
	}
	if r.Potential > 100 {
		t.Errorf("Potential = %d, want ≤ 100", r.Potential)
	}
}

func TestScore_Deterministic(t *testing.T) {
	a := Score(cleanCV, []string{"go"}, []string{"go", "kafka"})
	b := Score(cleanCV, []string{"go"}, []string{"go", "kafka"})
	if !reflect.DeepEqual(a, b) {
		t.Errorf("Score not deterministic:\n%+v\n%+v", a, b)
	}
}

func TestApplyReview_SetsContentQualityAndSuggestions(t *testing.T) {
	r := Score(cleanCV, cleanSkills, nil)
	base := r.Overall
	baseContent := catByID(t, r, "content_quality").Score
	r.ApplyReview(&Review{ContentQuality: 40, Suggestions: []string{"Quantify the billing project."}})

	// content_quality 40/100 × 15 = 6, replacing the proxy's 15 → Overall drops by 9.
	if got := catByID(t, r, "content_quality").Score; got != 6 {
		t.Errorf("content score after review = %d, want 6", got)
	}
	if r.Overall != base-(baseContent-6) {
		t.Errorf("Overall after review = %d, want %d", r.Overall, base-(baseContent-6))
	}
	if len(r.Suggestions) != 1 || r.Suggestions[0] != "Quantify the billing project." {
		t.Errorf("Suggestions = %v, want the single suggestion", r.Suggestions)
	}
}

func TestApplyReview_NilIsNoOp(t *testing.T) {
	r := Score(cleanCV, cleanSkills, nil)
	before := r
	r.ApplyReview(nil)
	if !reflect.DeepEqual(r, before) {
		t.Errorf("nil review changed the report")
	}
}
