package handler

import "testing"

func TestProfileFields_IncludesScreeningAnswers(t *testing.T) {
	p := autofillProfile{
		FullName:      "Ilya Strelov",
		NoticePeriod:  "30 days",
		DesiredSalary: "120000 USD/year",
	}
	got := profileFields(p)

	if got["notice_period"] != "30 days" {
		t.Errorf(`profileFields["notice_period"] = %q, want "30 days"`, got["notice_period"])
	}
	if got["desired_salary"] != "120000 USD/year" {
		t.Errorf(`profileFields["desired_salary"] = %q, want "120000 USD/year"`, got["desired_salary"])
	}
	// The existing identity fields must not regress.
	if got["full_name"] != "Ilya Strelov" {
		t.Errorf(`profileFields["full_name"] = %q, want "Ilya Strelov"`, got["full_name"])
	}
}
