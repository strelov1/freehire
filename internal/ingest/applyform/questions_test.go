package applyform

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

// loadStoredForm reads a payload exactly as apply_forms stores it — one real open
// posting's form per platform, sampled from production on 2026-10-31 — and stamps the
// provider the row carries beside it.
func loadStoredForm(t *testing.T, provider string) Form {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "forms", provider+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var f Form
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	f.Provider = provider
	return f
}

// Each fixture exercises what the count must leave out on that platform: the standard
// fields (required everywhere), optional employer questions, and — where the platform
// has them — consent boxes, hidden controls and the equal-opportunity survey.
func TestRequiredQuestionsOnRealForms(t *testing.T) {
	cases := []struct {
		provider string
		want     int
		why      string
	}{
		// first/last name, email, phone and the CV (as upload AND as pasted text) are all
		// required and all standard; "Salary expectations" is optional. Left: visa
		// sponsorship, EU work entitlement, GPA, office locations.
		{"greenhouse", 4, "four required question_<id> fields"},
		// _systemfield_ name/email/location/resume are standard. The employer's own
		// required "Phone Number" custom field counts — Ashby has no system phone field,
		// so this one IS the employer asking — as do GitHub and work authorization;
		// LinkedIn and Other URL are optional.
		{"ashby", 3, "phone number, GitHub, work authorization"},
		// Every standard Lever field is optional here, the consent box and the hidden
		// location are not questions, and of the two cards' eighteen questions only the
		// availability one in each is required.
		{"lever", 2, "one required question per card"},
		// Workable marks an employer question twice over: QA_ (written on the job) and
		// CA_ (configured once for the whole account). Nine CA_ fields and three QA_
		// fields are required; the standard profile (name, email, phone, address,
		// resume) is not counted.
		{"workable", 12, "nine CA_ and three QA_ required fields"},
		// A required cover letter is still a standard field; salary, hours and the
		// "where did you find us" select are the employer's.
		{"recruitee", 3, "three required numeric-id questions"},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			if got := loadStoredForm(t, tc.provider).RequiredQuestions(); got != tc.want {
				t.Errorf("RequiredQuestions() = %d, want %d (%s)", got, tc.want, tc.why)
			}
		})
	}
}

// The count is the required half of the list the job page shows under it, so the two
// cannot disagree.
func TestRequiredQuestionsMatchesDisplay(t *testing.T) {
	for _, provider := range []string{"greenhouse", "ashby", "lever", "workable", "recruitee"} {
		f := loadStoredForm(t, provider)
		n := 0
		for _, q := range f.ForDisplay().Questions {
			if q.Required {
				n++
			}
		}
		if got := f.RequiredQuestions(); got != n {
			t.Errorf("%s: RequiredQuestions() = %d, display lists %d required", provider, got, n)
		}
	}
}

func TestRequiredQuestionsExclusions(t *testing.T) {
	f := Form{Provider: "greenhouse", Fields: []Field{
		{ID: "first_name", Label: "First Name", Type: TypeText, Required: true},
		{ID: "preferred_name", Label: "Preferred First Name", Type: TypeText, Required: true},
		{ID: "gender", Label: "Gender", Type: TypeSelect, Required: true, Demographic: true},
		{ID: "consent[gdpr]", Label: "I agree", Type: TypeBoolean, Required: true},
		{ID: "question_1", Label: "", Type: TypeText, Required: true},
		{ID: "question_2", Label: "Notice", Type: TypeInfo, Required: true},
		{ID: "question_3", Label: "Why us?", Type: TypeTextarea, Required: true},
		{ID: "question_4", Label: "Salary?", Type: TypeText},
	}}
	if got := f.RequiredQuestions(); got != 1 {
		t.Errorf("RequiredQuestions() = %d, want 1 (only question_3)", got)
	}
	if got := (Form{Provider: "greenhouse"}).RequiredQuestions(); got != 0 {
		t.Errorf("empty form = %d, want 0: a captured form with nothing extra is a real zero", got)
	}
}

type fakeFormLister []db.ListApplyFormsForJobsRow

func (f fakeFormLister) ListApplyFormsForJobs(context.Context, []int64) ([]db.ListApplyFormsForJobsRow, error) {
	return f, nil
}

func TestQuestionCountsLeavesUnknownAbsent(t *testing.T) {
	gh, _ := json.Marshal(Form{Fields: []Field{
		{ID: "question_1", Label: "Why us?", Type: TypeTextarea, Required: true},
	}})
	empty, _ := json.Marshal(Form{})
	got, err := QuestionCounts(context.Background(), fakeFormLister{
		{JobID: 1, Provider: "greenhouse", Payload: gh},
		{JobID: 2, Provider: "lever", Payload: empty},
		{JobID: 3, Provider: "ashby", Payload: []byte("not json")},
	}, []int64{1, 2, 3, 4})
	if err != nil {
		t.Fatal(err)
	}
	if got[1] != 1 {
		t.Errorf("job 1 = %d, want 1", got[1])
	}
	if n, ok := got[2]; !ok || n != 0 {
		t.Errorf("job 2 = %d (present %v), want a present 0", n, ok)
	}
	for _, id := range []int64{3, 4} {
		if _, ok := got[id]; ok {
			t.Errorf("job %d has a count; an undecodable or missing form must stay unknown", id)
		}
	}
}
