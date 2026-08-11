package isoweek

import (
	"testing"
	"time"
)

func TestStart(t *testing.T) {
	cases := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{
			name: "Monday maps to itself",
			in:   time.Date(2026, 8, 10, 15, 30, 0, 0, time.UTC),
			want: time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "mid-week Wednesday maps to the preceding Monday",
			in:   time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC),
			want: time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "Sunday maps to the Monday six days prior",
			in:   time.Date(2026, 8, 16, 23, 59, 0, 0, time.UTC),
			want: time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Start(tc.in); !got.Equal(tc.want) {
				t.Errorf("Start(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
