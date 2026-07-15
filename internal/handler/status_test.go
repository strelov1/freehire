package handler

import (
	"testing"
	"time"
)

// statusNow is a fixed reference instant so freshness assertions don't couple to
// the wall clock.
var statusNow = time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

// fresh/stale success timestamps relative to statusNow and the 48h window.
var (
	freshSuccess = statusNow.Add(-1 * time.Hour)  // well within the window
	staleSuccess = statusNow.Add(-72 * time.Hour) // older than 48h
)

func TestDeriveStatus(t *testing.T) {
	cases := []struct {
		name string
		roll providerRollup
		want providerStatus
	}{
		{
			name: "all healthy and fresh is operational",
			roll: providerRollup{total: 100, healthy: 100, lastSuccess: freshSuccess},
			want: statusOperational,
		},
		{
			name: "exactly 90 percent healthy and fresh is operational",
			roll: providerRollup{total: 100, healthy: 90, lastSuccess: freshSuccess},
			want: statusOperational,
		},
		{
			name: "a minority failing is degraded",
			roll: providerRollup{total: 100, healthy: 80, lastSuccess: freshSuccess},
			want: statusDegraded,
		},
		{
			name: "almost all failing is down",
			roll: providerRollup{total: 100, healthy: 5, lastSuccess: freshSuccess},
			want: statusDown,
		},
		{
			name: "exactly 10 percent healthy is down",
			roll: providerRollup{total: 100, healthy: 10, lastSuccess: freshSuccess},
			want: statusDown,
		},
		{
			name: "healthy counts but stale success is down",
			roll: providerRollup{total: 100, healthy: 100, lastSuccess: staleSuccess},
			want: statusDown,
		},
		{
			name: "never succeeded is down",
			roll: providerRollup{total: 100, healthy: 100}, // zero lastSuccess = never
			want: statusDown,
		},
		{
			name: "no boards is down (defensive, avoids div-by-zero)",
			roll: providerRollup{total: 0, healthy: 0, lastSuccess: freshSuccess},
			want: statusDown,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deriveStatus(tc.roll, statusNow); got != tc.want {
				t.Errorf("deriveStatus(%+v) = %q, want %q", tc.roll, got, tc.want)
			}
		})
	}
}

func TestOverallStatus(t *testing.T) {
	cases := []struct {
		name string
		in   []providerStatus
		want providerStatus
	}{
		{"empty fleet is operational", nil, statusOperational},
		{"all operational is operational", []providerStatus{statusOperational, statusOperational}, statusOperational},
		{"any degraded drags to degraded", []providerStatus{statusOperational, statusDegraded, statusOperational}, statusDegraded},
		{"any down drags to down despite degraded", []providerStatus{statusOperational, statusDegraded, statusDown}, statusDown},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := overallStatus(tc.in); got != tc.want {
				t.Errorf("overallStatus(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
