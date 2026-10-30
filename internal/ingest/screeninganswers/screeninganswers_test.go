package screeninganswers

import (
	"reflect"
	"strings"
	"testing"
)

func ptrBool(b bool) *bool       { return &b }
func ptrInt(n int) *int          { return &n }
func ptrString(s string) *string { return &s }

func TestSanitize_NormalizesCountryCodesAndCurrency(t *testing.T) {
	a := Answers{
		AuthorizedCountries:   []string{"USA", "deu", " GB "},
		DesiredSalaryCurrency: ptrString("usd"),
	}
	a.Sanitize()

	wantCountries := []string{"us", "de", "gb"}
	if !reflect.DeepEqual(a.AuthorizedCountries, wantCountries) {
		t.Errorf("AuthorizedCountries = %v, want %v", a.AuthorizedCountries, wantCountries)
	}
	if a.DesiredSalaryCurrency == nil || *a.DesiredSalaryCurrency != "USD" {
		t.Errorf("DesiredSalaryCurrency = %v, want USD", a.DesiredSalaryCurrency)
	}
}

func TestSanitize_LeavesAnUnrecognizedCountryCodeForValidateToReject(t *testing.T) {
	// Sanitize must not silently drop a code Validate is supposed to reject — dropping it
	// here would let an invalid write through unrejected once Validate runs afterward.
	a := Answers{AuthorizedCountries: []string{"us", "not-a-code", "de"}}
	a.Sanitize()

	want := []string{"us", "not-a-code", "de"}
	if !reflect.DeepEqual(a.AuthorizedCountries, want) {
		t.Errorf("AuthorizedCountries = %v, want %v", a.AuthorizedCountries, want)
	}
	if err := a.Validate(); err == nil {
		t.Error("Validate() after Sanitize() = nil, want an error for the unrecognized code")
	}
}

func TestValidate_AcceptsFullyUnstatedAnswers(t *testing.T) {
	if err := (Answers{}).Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil for a fully unstated record", err)
	}
}

func TestValidate_AcceptsAFullySetRecord(t *testing.T) {
	a := Answers{
		AuthorizedCountries:   []string{"us", "de"},
		VisaSponsorshipNeeded: ptrBool(true),
		DesiredSalaryAmount:   ptrInt(120000),
		DesiredSalaryCurrency: ptrString("USD"),
		DesiredSalaryPeriod:   ptrString("year"),
		NoticePeriodDays:      ptrInt(30),
		WillingToRelocate:     ptrBool(false),
		Age18OrOlder:          ptrBool(true),
	}
	if err := a.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestValidate_RejectsMalformedCurrency(t *testing.T) {
	cases := []string{"US", "USDD", "usd", "12D"}
	for _, currency := range cases {
		a := Answers{DesiredSalaryCurrency: ptrString(currency)}
		if err := a.Validate(); err == nil {
			t.Errorf("Validate() with currency %q = nil, want an error", currency)
		}
	}
}

func TestValidate_RejectsInvalidPeriod(t *testing.T) {
	a := Answers{DesiredSalaryPeriod: ptrString("annually")}
	if err := a.Validate(); err == nil {
		t.Error("Validate() with an invalid period = nil, want an error")
	}
}

func TestValidate_AcceptsEveryVocabPeriod(t *testing.T) {
	for _, period := range []string{"year", "month", "day", "hour"} {
		a := Answers{DesiredSalaryPeriod: ptrString(period)}
		if err := a.Validate(); err != nil {
			t.Errorf("Validate() with period %q = %v, want nil", period, err)
		}
	}
}

func TestValidate_RejectsNonPositiveSalaryAmount(t *testing.T) {
	for _, amount := range []int{0, -1} {
		a := Answers{DesiredSalaryAmount: ptrInt(amount)}
		if err := a.Validate(); err == nil {
			t.Errorf("Validate() with salary amount %d = nil, want an error", amount)
		}
	}
}

func TestValidate_RejectsNegativeNoticePeriod(t *testing.T) {
	a := Answers{NoticePeriodDays: ptrInt(-1)}
	if err := a.Validate(); err == nil {
		t.Error("Validate() with a negative notice period = nil, want an error")
	}
}

func TestMerge_PartialUpdateLeavesOtherFieldsUnchanged(t *testing.T) {
	existing := Answers{
		DesiredSalaryAmount:   ptrInt(100000),
		DesiredSalaryCurrency: ptrString("USD"),
		WillingToRelocate:     ptrBool(false),
	}
	update := Answers{WillingToRelocate: ptrBool(true)}

	got := Merge(existing, update)

	if got.DesiredSalaryAmount == nil || *got.DesiredSalaryAmount != 100000 {
		t.Errorf("DesiredSalaryAmount = %v, want 100000 (untouched)", got.DesiredSalaryAmount)
	}
	if got.DesiredSalaryCurrency == nil || *got.DesiredSalaryCurrency != "USD" {
		t.Errorf("DesiredSalaryCurrency = %v, want USD (untouched)", got.DesiredSalaryCurrency)
	}
	if got.WillingToRelocate == nil || *got.WillingToRelocate != true {
		t.Errorf("WillingToRelocate = %v, want true (updated)", got.WillingToRelocate)
	}
}

func TestMerge_EmptyUpdateChangesNothing(t *testing.T) {
	existing := Answers{
		AuthorizedCountries: []string{"us"},
		NoticePeriodDays:    ptrInt(14),
	}

	got := Merge(existing, Answers{})

	if !reflect.DeepEqual(got, existing) {
		t.Errorf("Merge(existing, empty) = %+v, want %+v (unchanged)", got, existing)
	}
}

func TestMerge_UpdateOverwritesAuthorizedCountries(t *testing.T) {
	existing := Answers{AuthorizedCountries: []string{"us"}}
	update := Answers{AuthorizedCountries: []string{"de", "fr"}}

	got := Merge(existing, update)

	want := []string{"de", "fr"}
	if !reflect.DeepEqual(got.AuthorizedCountries, want) {
		t.Errorf("AuthorizedCountries = %v, want %v", got.AuthorizedCountries, want)
	}
}

func TestMerge_ClearReturnsAFieldToUnstated(t *testing.T) {
	existing := Answers{
		AuthorizedCountries: []string{"de", "pl"},
		DesiredSalaryAmount: ptrInt(120000),
		WillingToRelocate:   ptrBool(true),
	}

	got := Merge(existing, Answers{}, FieldAuthorizedCountries, FieldDesiredSalaryAmount)

	if got.AuthorizedCountries != nil {
		t.Errorf("AuthorizedCountries = %v, want nil (cleared)", got.AuthorizedCountries)
	}
	if got.DesiredSalaryAmount != nil {
		t.Errorf("DesiredSalaryAmount = %v, want nil (cleared)", *got.DesiredSalaryAmount)
	}
	if got.WillingToRelocate == nil || !*got.WillingToRelocate {
		t.Errorf("WillingToRelocate = %v, want true (untouched)", got.WillingToRelocate)
	}
}

func TestMerge_ClearsEveryField(t *testing.T) {
	full := Answers{
		AuthorizedCountries:   []string{"de"},
		VisaSponsorshipNeeded: ptrBool(true),
		DesiredSalaryAmount:   ptrInt(1),
		DesiredSalaryCurrency: ptrString("USD"),
		DesiredSalaryPeriod:   ptrString("year"),
		NoticePeriodDays:      ptrInt(30),
		WillingToRelocate:     ptrBool(true),
		Age18OrOlder:          ptrBool(true),
		PrivacyConsent:        ptrBool(true),
	}

	got := Merge(full, Answers{}, Fields()...)

	if !reflect.DeepEqual(got, Answers{}) {
		t.Errorf("Merge clearing every field = %+v, want an empty record", got)
	}
}

func TestParseField(t *testing.T) {
	if f, err := ParseField("desired_salary_amount"); err != nil || f != FieldDesiredSalaryAmount {
		t.Errorf("ParseField(desired_salary_amount) = %q, %v", f, err)
	}
	if _, err := ParseField("salary"); err == nil {
		t.Error("ParseField(salary) = nil error, want an unknown-field error")
	}
}

// A field added to Answers but not to Fields could be stated and never withdrawn, with
// nothing reporting it. Every JSON key Answers carries must parse as a Field.
func TestFields_CoverEveryAnswer(t *testing.T) {
	typ := reflect.TypeOf(Answers{})
	if typ.NumField() != len(Fields()) {
		t.Errorf("Answers has %d fields, Fields() lists %d", typ.NumField(), len(Fields()))
	}
	for i := range typ.NumField() {
		name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		if _, err := ParseField(name); err != nil {
			t.Errorf("Answers.%s (%q) is not a Field", typ.Field(i).Name, name)
		}
	}
}
