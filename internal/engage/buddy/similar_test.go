package buddy

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
)

func TestClampLimit(t *testing.T) {
	for in, want := range map[int]int{-3: DefaultSimilarLimit, 0: DefaultSimilarLimit, 1: 1, 50: 50, 51: MaxSimilarLimit, 9999: MaxSimilarLimit} {
		if got := clampLimit(in); got != want {
			t.Errorf("clampLimit(%d) = %d, want %d", in, got, want)
		}
	}
}

// The weights are a first guess, but the ordering they must preserve is not: a shared
// category beats any single other signal, so the first thing a viewer reads is "same
// line of work". The score itself is computed in SQL; the integration tests pin it.
func TestWeightsOrdering(t *testing.T) {
	if weightCategory <= weightRegions || weightCategory <= weightCompany || weightCategory <= weightCountries {
		t.Errorf("category weight %d must exceed every single other signal", weightCategory)
	}
	if weightCountries <= weightRegions {
		t.Errorf("countries weight %d must exceed regions weight %d", weightCountries, weightRegions)
	}
	if weightRegions <= 0 {
		t.Error("every signal must add points, or a zero-weight signal would never rank")
	}
}

func TestCardSummary(t *testing.T) {
	cv := resumeextract.Structured{Experience: []resumeextract.Experience{{Title: "Senior Backend Engineer", Current: true}}}
	got := cardSummary(SimilarRow{CV: cv, Countries: []string{"de", "pl"}})
	if got.Category != "backend" || got.Seniority == "" || got.Country != "de" {
		t.Errorf("summary = %+v", got)
	}
	if empty := cardSummary(SimilarRow{}); empty != (CardSummary{}) {
		t.Errorf("empty row summary = %+v", empty)
	}
}

type stubRepo struct {
	Repository
	member Member
	found  bool
}

func (s stubRepo) Member(context.Context, int64) (Member, bool, error) { return s.member, s.found, nil }

func TestSimilarRefusesNonMember(t *testing.T) {
	for name, r := range map[string]stubRepo{"unknown": {}, "left": {member: Member{Active: false}, found: true}} {
		if _, err := New(r).Similar(context.Background(), 1, 10, 0); !errors.Is(err, ErrNotMember) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}
