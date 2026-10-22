package answertopic

import "testing"

func TestRecallable_RefusesWorkAuthorizationOnly(t *testing.T) {
	refused := []string{
		"Are you authorized to work in the country in which this position is located?",
		"Do you have the right to work in the UK?",
		"Will you now or in the future require visa sponsorship?",
	}
	for _, q := range refused {
		if Recallable(q) {
			t.Errorf("Recallable(%q) = true, want a work-authorization question refused", q)
		}
	}
	allowed := []string{"What is your desired salary?", "Gender", "Have you interviewed with us before?"}
	for _, q := range allowed {
		if !Recallable(q) {
			t.Errorf("Recallable(%q) = false, want it recalled", q)
		}
	}
}
