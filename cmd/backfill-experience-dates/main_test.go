package main

import (
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
)

func TestParseOrFallback(t *testing.T) {
	createdAt := time.Date(2022, 5, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name         string
		raw          string
		want         *perioddate.PeriodDate
		wantFellBack bool
	}{
		{"parses cleanly", "October 2018", &perioddate.PeriodDate{Year: 2018, Month: 10}, false},
		{"bare year parses", "2024", &perioddate.PeriodDate{Year: 2024}, false},
		{"empty is a real absence, no fallback", "", nil, false},
		{"whitespace-only is a real absence, no fallback", "   ", nil, false},
		{"Present is a real absence, no fallback", "Present", nil, false},
		{"present-label case/whitespace insensitive", "  current  ", nil, false},
		{"garbled text falls back to created_at year", "sometime last year", &perioddate.PeriodDate{Year: 2022}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, fellBack := parseOrFallback(tc.raw, createdAt)
			if (got == nil) != (tc.want == nil) {
				t.Fatalf("parseOrFallback(%q) = %+v, want %+v", tc.raw, got, tc.want)
			}
			if got != nil && *got != *tc.want {
				t.Fatalf("parseOrFallback(%q) = %+v, want %+v", tc.raw, got, tc.want)
			}
			if fellBack != tc.wantFellBack {
				t.Errorf("parseOrFallback(%q) fellBack = %v, want %v", tc.raw, fellBack, tc.wantFellBack)
			}
		})
	}
}
