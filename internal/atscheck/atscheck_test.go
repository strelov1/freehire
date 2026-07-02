package atscheck

import (
	"reflect"
	"strings"
	"testing"
)

// cleanCV is a realistic plain-text CV that should pass every structural check
// (kept > the length floor so the length check passes honestly).
const cleanCV = `Ilya Ivanov
ilya@example.com  +1 415 555 0134  San Francisco, CA

Summary
Senior backend engineer with eight years building high-throughput distributed
systems and data platforms. Comfortable owning services end to end, from design
and implementation through on-call and cost optimization.

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

func check(t *testing.T, r Report, id string) Check {
	t.Helper()
	for _, c := range r.Checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no check %q in %+v", id, r.Checks)
	return Check{}
}

func TestScore_ScannedCVFailsMachineReadable(t *testing.T) {
	r := Score("   ", nil, nil)
	if got := check(t, r, "machine_readable"); got.Status != StatusFail {
		t.Errorf("machine_readable = %s, want fail", got.Status)
	}
	if r.Readability > 30 {
		t.Errorf("readability = %d, want low for a scan", r.Readability)
	}
}

func TestScore_CleanCVPassesStructure(t *testing.T) {
	r := Score(cleanCV, nil, nil)
	for _, id := range []string{"machine_readable", "contact", "sections", "dates", "length", "bullets"} {
		if got := check(t, r, id); got.Status != StatusPass {
			t.Errorf("%s = %s, want pass", id, got.Status)
		}
	}
	if r.Readability < 80 {
		t.Errorf("readability = %d, want high for a clean CV", r.Readability)
	}
}

func TestScore_KeywordMatchFromRoleSkills(t *testing.T) {
	// CV has go + kafka; role wants go, kubernetes, kafka → 2/3 = 67, kubernetes missing.
	r := Score(cleanCV, []string{"go", "kafka"}, []string{"go", "kubernetes", "kafka"})
	if r.KeywordMatch != 67 {
		t.Errorf("KeywordMatch = %d, want 67", r.KeywordMatch)
	}
	if fix := check(t, r, "keyword_match").Fix; !strings.Contains(fix, "kubernetes") {
		t.Errorf("keyword_match fix = %q, want it to name kubernetes", fix)
	}
}

func TestScore_KeywordMatchZeroRoleSkills(t *testing.T) {
	r := Score(cleanCV, []string{"go"}, nil)
	if r.KeywordMatch != 0 {
		t.Errorf("KeywordMatch = %d, want 0 when no role skills", r.KeywordMatch)
	}
}

func TestScore_OverallBlendsAndClamps(t *testing.T) {
	r := Score(cleanCV, []string{"go", "kafka"}, []string{"go", "kubernetes", "kafka"})
	if r.Overall < 0 || r.Overall > 100 {
		t.Errorf("Overall = %d, want in [0,100]", r.Overall)
	}
	// A clean CV with partial keyword match should land between the two sub-scores' extremes.
	if r.Overall == 0 {
		t.Errorf("Overall = 0, want a blended non-zero score")
	}
}

func TestScore_Deterministic(t *testing.T) {
	a := Score(cleanCV, []string{"go"}, []string{"go", "kafka"})
	b := Score(cleanCV, []string{"go"}, []string{"go", "kafka"})
	if !reflect.DeepEqual(a, b) {
		t.Errorf("Score not deterministic:\n%+v\n%+v", a, b)
	}
}
