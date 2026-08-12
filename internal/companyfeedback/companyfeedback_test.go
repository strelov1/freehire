package companyfeedback

import "testing"

func TestValidFeedbackType(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"interview", true},
		{"culture", true},
		{"compensation", true},
		{"management", true},
		{"work_life_balance", true},
		{"career_growth", true},
		{"other", true},
		{"", false},
		{"salary", false},
		{"INTERVIEW", false},
	}
	for _, c := range cases {
		if got := validFeedbackType(c.in); got != c.want {
			t.Errorf("validFeedbackType(%q) = %v; want %v", c.in, got, c.want)
		}
	}
}

func TestValidReportReason(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"spam", true},
		{"offensive", true},
		{"false_information", true},
		{"other", true},
		{"", false},
		{"no_response", false},
		{"SPAM", false},
	}
	for _, c := range cases {
		if got := validReportReason(c.in); got != c.want {
			t.Errorf("validReportReason(%q) = %v; want %v", c.in, got, c.want)
		}
	}
}
