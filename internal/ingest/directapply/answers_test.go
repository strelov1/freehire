package directapply

import (
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/applyform"
)

func testForm(t *testing.T) applyform.Form {
	t.Helper()
	form, err := applyform.FreehireForm([]applyform.EmployerQuestion{
		{Label: "EU work permit?", Type: applyform.TypeBoolean, Required: true},
		{Label: "Stack", Type: applyform.TypeSelect, Options: []string{"Go", "Rust"}},
		{Label: "Anything else?", Type: applyform.TypeText},
	})
	if err != nil {
		t.Fatal(err)
	}
	return form
}

func TestCheckAnswers_SnapshotsTheQuestionsInFormOrder(t *testing.T) {
	got, err := checkAnswers(testForm(t), map[string]string{"q2": " Rust ", "q1": "true"})
	if err != nil {
		t.Fatalf("checkAnswers: %v", err)
	}
	want := []Answer{
		{ID: "q1", Label: "EU work permit?", Type: applyform.TypeBoolean, Answer: "true"},
		{ID: "q2", Label: "Stack", Type: applyform.TypeSelect, Answer: "Rust"},
	}
	if len(got) != len(want) {
		t.Fatalf("answers = %+v, want %+v (an unanswered optional question is left out)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("answer %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestCheckAnswers_Refusals(t *testing.T) {
	cases := map[string]struct {
		answers map[string]string
		names   string // the question the error must name, if any
	}{
		"required unanswered":        {map[string]string{"q2": "Go"}, "EU work permit?"},
		"required blank":             {map[string]string{"q1": "  "}, "EU work permit?"},
		"boolean not a boolean":      {map[string]string{"q1": "yes"}, "EU work permit?"},
		"select outside its options": {map[string]string{"q1": "false", "q2": "Java"}, "Stack"},
		"unknown question":           {map[string]string{"q1": "true", "q9": "x"}, ""},
		"text too long":              {map[string]string{"q1": "true", "q3": strings.Repeat("x", maxAnswer+1)}, "Anything else?"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := checkAnswers(testForm(t), tc.answers)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
			if tc.names != "" && !strings.Contains(err.Error(), tc.names) {
				t.Errorf("err = %q, want it to name %q", err, tc.names)
			}
		})
	}
}

func TestCheckNote(t *testing.T) {
	if got, err := checkNote("  hello  "); err != nil || got != "hello" {
		t.Errorf("checkNote = %q, %v", got, err)
	}
	if _, err := checkNote(strings.Repeat("ж", maxNote+1)); !errors.Is(err, ErrInvalid) {
		t.Errorf("an over-long note: err = %v, want ErrInvalid", err)
	}
}
