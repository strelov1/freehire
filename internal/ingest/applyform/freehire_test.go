package applyform

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestFreehireForm_BuildsTheEmployersQuestions(t *testing.T) {
	form, err := FreehireForm([]EmployerQuestion{
		{Label: "  Do you hold an EU work permit? ", Type: TypeBoolean, Required: true},
		{Label: "Preferred stack", Type: TypeSelect, Options: []string{"Go", " Rust "}},
		{Label: "Anything else?", Type: TypeText},
	})
	if err != nil {
		t.Fatalf("FreehireForm: %v", err)
	}
	if form.Provider != ProviderFreehire {
		t.Errorf("Provider = %q, want %q", form.Provider, ProviderFreehire)
	}
	if len(form.Fields) != 3 {
		t.Fatalf("fields = %d, want 3", len(form.Fields))
	}
	first := form.Fields[0]
	if first.ID != "q1" || first.Label != "Do you hold an EU work permit?" || first.Type != TypeBoolean || !first.Required {
		t.Errorf("first field = %+v", first)
	}
	if first.RawType != string(TypeBoolean) {
		t.Errorf("RawType = %q, want the type itself — the employer's word is ours", first.RawType)
	}
	sel := form.Fields[1]
	if sel.ID != "q2" || !slices.Equal(sel.Options, []Option{{Label: "Go", Value: "Go"}, {Label: "Rust", Value: "Rust"}}) {
		t.Errorf("select field = %+v", sel)
	}
	if form.Fields[2].ID != "q3" || form.Fields[2].Required {
		t.Errorf("third field = %+v", form.Fields[2])
	}
}

func TestFreehireForm_NoQuestionsIsAForm(t *testing.T) {
	form, err := FreehireForm(nil)
	if err != nil {
		t.Fatalf("FreehireForm(nil): %v", err)
	}
	if form.Provider != ProviderFreehire || len(form.Fields) != 0 {
		t.Errorf("form = %+v, want an empty freehire form", form)
	}
}

func TestFreehireForm_RefusesMalformedQuestions(t *testing.T) {
	eleven := make([]EmployerQuestion, 11)
	for i := range eleven {
		eleven[i] = EmployerQuestion{Label: "Q", Type: TypeText}
	}
	manyOptions := make([]string, 21)
	for i := range manyOptions {
		manyOptions[i] = strings.Repeat("x", i+1)
	}
	cases := map[string][]EmployerQuestion{
		"more than ten":         eleven,
		"blank label":           {{Label: "  ", Type: TypeText}},
		"label too long":        {{Label: strings.Repeat("x", 301), Type: TypeText}},
		"unsupported type":      {{Label: "CV", Type: TypeFile}},
		"no type":               {{Label: "Q"}},
		"select with one":       {{Label: "Q", Type: TypeSelect, Options: []string{"A"}}},
		"select with too many":  {{Label: "Q", Type: TypeSelect, Options: manyOptions}},
		"select duplicate":      {{Label: "Q", Type: TypeSelect, Options: []string{"A", " A"}}},
		"select blank option":   {{Label: "Q", Type: TypeSelect, Options: []string{"A", " "}}},
		"options on a non-list": {{Label: "Q", Type: TypeBoolean, Options: []string{"A", "B"}}},
	}
	for name, qs := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := FreehireForm(qs); !errors.Is(err, ErrInvalidQuestions) {
				t.Errorf("err = %v, want ErrInvalidQuestions", err)
			}
		})
	}
}

// An employer's questions are all the employer's own: none folds into the standard
// "basics" line the way a captured form's name/email/CV fields do, and each keeps its
// required flag on the job page.
func TestFreehireForm_DisplaysEveryQuestion(t *testing.T) {
	form, err := FreehireForm([]EmployerQuestion{
		{Label: "Email", Type: TypeText, Required: true},
		{Label: "EU work permit?", Type: TypeBoolean},
	})
	if err != nil {
		t.Fatal(err)
	}
	d := form.ForDisplay()
	if len(d.Basics) != 0 {
		t.Errorf("Basics = %v, want none — an employer question is never a standard field", d.Basics)
	}
	if len(d.Questions) != 2 || d.Questions[0].Text != "Email" || !d.Questions[0].Required || d.Questions[1].Required {
		t.Errorf("Questions = %+v", d.Questions)
	}
}
