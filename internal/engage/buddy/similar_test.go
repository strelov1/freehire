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
		if _, err := New(r, nil).Similar(context.Background(), 1, 10, 0); !errors.Is(err, ErrNotMember) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

type similarOffsetRepo struct {
	stubRepo
	offset int
}

func (r *similarOffsetRepo) Similar(_ context.Context, _ int64, _, offset int) ([]SimilarRow, error) {
	r.offset = offset
	return nil, nil
}

func TestSimilarClampsNegativeOffsets(t *testing.T) {
	for _, tc := range []struct{ offset, want int }{{-5, 0}, {0, 0}, {12, 12}} {
		r := &similarOffsetRepo{stubRepo: stubRepo{member: Member{Active: true}, found: true}}
		if _, err := New(r, nil).Similar(context.Background(), 1, 10, tc.offset); err != nil {
			t.Fatal(err)
		}
		if r.offset != tc.want {
			t.Errorf("offset %d reached repository as %d, want %d", tc.offset, r.offset, tc.want)
		}
	}
}
