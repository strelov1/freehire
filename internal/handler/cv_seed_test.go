package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/resumeextract"
)

type stubHistory struct {
	history []resumeextract.Experience
	err     error
}

func (s stubHistory) WorkHistory(context.Context, int64) ([]resumeextract.Experience, error) {
	return s.history, s.err
}

// bankedSeeder must satisfy cv.Seeder — that is what keeps internal/cv ignorant of the bank.
var _ cv.Seeder = bankedSeeder{}

// The loop closes here: evidence confirmed while tailoring for one vacancy appears in the
// base CV the candidate creates next, even though it is in no uploaded file.
func TestBankedSeederTakesExperienceFromTheBank(t *testing.T) {
	seeder := bankedSeeder{
		resume: fakeStructuredResume{ok: true, ret: resumeextract.Structured{
			FullName:  "Ada Lovelace",
			Headline:  "Staff Backend Engineer",
			Education: []resumeextract.Education{{Degree: "BSc"}},
			// The structure's own copy is stale and must be replaced, not merged.
			Experience: []resumeextract.Experience{{Company: "STALE"}},
		}},
		bank: stubHistory{history: []resumeextract.Experience{
			{Company: "RingCentral", Title: "SWE", Highlights: []string{"Confirmed in chat"}},
		}},
	}

	st, ok, err := seeder.Structured(context.Background(), 1)
	if err != nil || !ok {
		t.Fatalf("Structured = ok:%v err:%v, want a seedable structure", ok, err)
	}
	if len(st.Experience) != 1 || st.Experience[0].Company != "RingCentral" {
		t.Errorf("experience = %+v, want the banked history replacing the structure's", st.Experience)
	}
	if st.FullName != "Ada Lovelace" || len(st.Education) != 1 {
		t.Errorf("the file-owned sections were lost: %+v", st)
	}

	// And the mapping the CV builder already had still applies, unchanged.
	doc := cv.Seed(st)
	if len(doc.Experience) != 1 || doc.Experience[0].Company != "RingCentral" {
		t.Errorf("seeded document experience = %+v", doc.Experience)
	}
	if len(doc.Experience[0].Bullets) != 1 {
		t.Errorf("the banked claim did not become a bullet: %+v", doc.Experience[0])
	}
}

// A candidate with a bank and no parsed file can still bootstrap a CV — which is the whole
// point of the bank outliving the artifact.
func TestBankedSeederWorksWithNoStructure(t *testing.T) {
	seeder := bankedSeeder{
		resume: fakeStructuredResume{ok: false},
		bank:   stubHistory{history: []resumeextract.Experience{{Company: "RingCentral"}}},
	}

	st, ok, err := seeder.Structured(context.Background(), 1)
	if err != nil || !ok {
		t.Fatalf("Structured = ok:%v err:%v, want seedable from the bank alone", ok, err)
	}
	if len(st.Experience) != 1 {
		t.Errorf("experience = %+v, want the banked role", st.Experience)
	}
}

// Nothing known anywhere is what the tailoring bootstrap turns into "add a résumé first".
func TestBankedSeederReportsNothingToSeedFrom(t *testing.T) {
	seeder := bankedSeeder{resume: fakeStructuredResume{ok: false}, bank: stubHistory{}}

	_, ok, err := seeder.Structured(context.Background(), 1)
	if err != nil {
		t.Fatalf("Structured: %v", err)
	}
	if ok {
		t.Error("reported something to seed from with an empty bank and no structure")
	}
}

// A failing bank costs the work history, not the whole bootstrap: the file-owned sections
// are still worth seeding from.
func TestBankedSeederSurvivesAFailingBank(t *testing.T) {
	seeder := bankedSeeder{
		resume: fakeStructuredResume{ok: true, ret: resumeextract.Structured{Headline: "Staff Backend Engineer"}},
		bank:   stubHistory{err: errors.New("database down")},
	}

	st, ok, err := seeder.Structured(context.Background(), 1)
	if err != nil || !ok {
		t.Fatalf("Structured = ok:%v err:%v, want the structure still seedable", ok, err)
	}
	if len(st.Experience) != 0 {
		t.Errorf("experience = %+v, want none from a failing bank", st.Experience)
	}
}
