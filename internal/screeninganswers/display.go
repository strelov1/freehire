package screeninganswers

import (
	"strconv"
	"strings"
)

// AutofillFields projects the record into the human-readable strings the browser
// extension's autofill agent grounds its plan in — the same shape the identity fields
// (name, email, phone, …) already reach the agent as. A field the candidate has not
// stated is omitted rather than defaulted, so the agent's own "declining is correct" rule
// still applies to it exactly as it did before this store existed.
func (a Answers) AutofillFields() map[string]string {
	fields := make(map[string]string, 6)
	if len(a.AuthorizedCountries) > 0 {
		codes := make([]string, len(a.AuthorizedCountries))
		for i, c := range a.AuthorizedCountries {
			codes[i] = strings.ToUpper(c)
		}
		fields["authorized_countries"] = strings.Join(codes, ", ")
	}
	if a.VisaSponsorshipNeeded != nil {
		fields["visa_sponsorship_needed"] = yesNo(*a.VisaSponsorshipNeeded)
	}
	if salary := formatDesiredSalary(a); salary != "" {
		fields["desired_salary"] = salary
	}
	if a.NoticePeriodDays != nil {
		fields["notice_period"] = strconv.Itoa(*a.NoticePeriodDays) + " days"
	}
	if a.WillingToRelocate != nil {
		fields["willing_to_relocate"] = yesNo(*a.WillingToRelocate)
	}
	if a.Age18OrOlder != nil {
		fields["age_18_or_older"] = yesNo(*a.Age18OrOlder)
	}
	return fields
}

// formatDesiredSalary builds the desired-salary string from whichever of amount,
// currency and period the candidate stated, in that order — "120000 USD/year" with
// everything, degrading gracefully down to "120000" with only the amount. Empty when the
// amount itself is unstated: a currency or period with no figure names nothing to fill.
func formatDesiredSalary(a Answers) string {
	if a.DesiredSalaryAmount == nil {
		return ""
	}
	s := strconv.Itoa(*a.DesiredSalaryAmount)
	if a.DesiredSalaryCurrency != nil {
		s += " " + *a.DesiredSalaryCurrency
	}
	if a.DesiredSalaryPeriod != nil {
		s += "/" + *a.DesiredSalaryPeriod
	}
	return s
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
