package screeninganswers_test

import (
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/screeninganswers"
)

func TestAutofillFields_EmptyRecordYieldsNoFields(t *testing.T) {
	got := screeninganswers.Answers{}.AutofillFields()
	if len(got) != 0 {
		t.Errorf("AutofillFields() = %v, want empty", got)
	}
}

func TestAutofillFields_OnlyStatedFieldsAppear(t *testing.T) {
	days := 30
	got := screeninganswers.Answers{NoticePeriodDays: &days}.AutofillFields()
	want := map[string]string{"notice_period": "30 days"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AutofillFields() = %v, want %v", got, want)
	}
}

func TestAutofillFields_BooleansFormatAsYesNo(t *testing.T) {
	yes, no := true, false
	got := screeninganswers.Answers{
		VisaSponsorshipNeeded: &yes,
		WillingToRelocate:     &no,
		Age18OrOlder:          &yes,
	}.AutofillFields()
	want := map[string]string{
		"visa_sponsorship_needed": "yes",
		"willing_to_relocate":     "no",
		"age_18_or_older":         "yes",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AutofillFields() = %v, want %v", got, want)
	}
}

func TestAutofillFields_AuthorizedCountriesJoinsCodesUppercase(t *testing.T) {
	got := screeninganswers.Answers{AuthorizedCountries: []string{"us", "de"}}.AutofillFields()
	want := map[string]string{"authorized_countries": "US, DE"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AutofillFields() = %v, want %v", got, want)
	}
}

func TestAutofillFields_DesiredSalaryFormatsWithAvailableParts(t *testing.T) {
	amount, currency, period := 120000, "USD", "year"
	cases := []struct {
		name string
		a    screeninganswers.Answers
		want string
	}{
		{"amount only", screeninganswers.Answers{DesiredSalaryAmount: &amount}, "120000"},
		{"amount + currency", screeninganswers.Answers{DesiredSalaryAmount: &amount, DesiredSalaryCurrency: &currency}, "120000 USD"},
		{"amount + currency + period", screeninganswers.Answers{DesiredSalaryAmount: &amount, DesiredSalaryCurrency: &currency, DesiredSalaryPeriod: &period}, "120000 USD/year"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.a.AutofillFields()
			if got["desired_salary"] != tc.want {
				t.Errorf("desired_salary = %q, want %q", got["desired_salary"], tc.want)
			}
		})
	}
}

func TestAutofillFields_NoSalaryAmountYieldsNoDesiredSalaryField(t *testing.T) {
	currency := "USD"
	got := screeninganswers.Answers{DesiredSalaryCurrency: &currency}.AutofillFields()
	if _, ok := got["desired_salary"]; ok {
		t.Errorf("desired_salary present with no amount stated: %v", got)
	}
}
