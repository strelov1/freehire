package classify

import (
	"strings"
	"testing"
)

func TestFoldTerm(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Back-End", "back-end"},
		{"back end", "back-end"},
		{"ёлка", "елка"},
		{"ML / AI", "ml-ai"},
		{"  ", ""},
		{"C++", "c"},
	}
	for _, tc := range cases {
		if got := FoldTerm(tc.in); got != tc.want {
			t.Errorf("FoldTerm(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPlanGap(t *testing.T) {
	pending := GapRow{Status: "pending", Raw: "Back-End", Count: 2, SubmittedBy: 7}
	got := PlanGap("back end", 0, []GapRow{pending})
	if got.Refuse != "" || got.Op != GapIncrement || got.Status != "pending" || got.Normalized != "back-end" {
		t.Fatalf("pending = %+v", got)
	}

	accepted := GapRow{Status: "accepted", Raw: "golang", Count: 1, SubmittedBy: 3}
	got = PlanGap("golang", 0, []GapRow{accepted})
	if got.Op != GapIncrement || got.Status != "accepted" {
		t.Fatalf("accepted = %+v", got)
	}

	rejected := GapRow{Status: "rejected", Raw: "sql ninja", Count: 1, SubmittedBy: 3}
	got = PlanGap("SQL Ninja", 0, []GapRow{rejected})
	if got.Op != GapInsert || got.Status != "pending" {
		t.Fatalf("rejected = %+v", got)
	}

	if got := PlanGap("   ", 0, nil); got.Refuse != GapRefuseEmpty {
		t.Fatalf("empty = %+v", got)
	}
	long := strings.Repeat("a", 81)
	if got := PlanGap(long, 0, nil); got.Refuse != GapRefuseLong {
		t.Fatalf("long = %+v", got)
	}
	if got := PlanGap("golang", 20, nil); got.Refuse != GapRefuseCap {
		t.Fatalf("cap = %+v", got)
	}
}
