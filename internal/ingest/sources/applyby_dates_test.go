package sources

import "testing"

// time.Date rolls 31.02 over to March; the day-month-year readers must refuse it instead,
// so a bad page date cannot shadow a valid fallback or close a posting early.
func TestApplyByDateReadersRejectImpossibleDays(t *testing.T) {
	if _, ok := essDayMonthYear("31. 2. 2026"); ok {
		t.Error("ess accepted 31. 2. 2026")
	}
	if _, ok := infostudDeadline("31.02.2026."); ok {
		t.Error("infostud accepted 31.02.2026.")
	}
	if d := nszDate("29.10.2026. - 31.02.2026.", 6); d != nil {
		t.Errorf("nsz accepted 31.02.2026: %v", d)
	}
	if _, ok := infostudDeadline("28.02.2026."); !ok {
		t.Error("infostud rejected a real date")
	}
}
