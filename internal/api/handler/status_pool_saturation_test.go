package handler

import "testing"

// TestPoolPressureIsReportedNotJudged pins the decision that cost a defect to notice.
//
// An earlier draft fed this fraction into deriveSiteStatus, so a pool at 90% read
// `degraded`. That is wrong here, and permanently so: StartSiteStatusSampler takes ONE
// reading every five minutes and RecordSiteStatusSample keeps the day's WORST severity,
// while the live pool — sampled every 5s against the healthy production site — reads
// 9/10 and 10/10 inside the same two minutes it otherwise spends at 0/10. Real traffic is
// bursty and touching the ceiling is ordinary, so one unlucky sample would have painted a
// whole day degraded and the 90-day history strip would have gone yellow for good.
//
// The number is still reported, and the Grafana rule averages it over five minutes to
// reach a verdict — 0.125-0.235 healthy against the outage's sustained 1.0. That needs
// history this process does not keep.
func TestPoolPressureIsReportedNotJudged(t *testing.T) {
	// Fully exhausted, no errors, quiet: the exact shape of the 2026-09-14 outage as this
	// process could see it, and still not a verdict from here.
	if got := deriveSiteStatus(true, 0, 0); got != statusOperational {
		t.Errorf("deriveSiteStatus with an exhausted pool = %q, want %q — the pool must not "+
			"be an input, or one burst paints the whole day", got, statusOperational)
	}
}

// TestPoolPressureReadsTheFraction covers the arithmetic and the one case that is not
// arithmetic: a pool reporting no capacity. "I cannot measure this" must not render as
// "everything is held", and a division by zero would.
func TestPoolPressureReadsTheFraction(t *testing.T) {
	cases := []struct {
		name               string
		acquired, maxConns int32
		want               float64
	}{
		{"idle", 0, 10, 0},
		{"half", 5, 10, 0.5},
		{"exhausted", 10, 10, 1},
		{"no capacity reported", 0, 0, 0},
		{"no capacity, yet acquired", 3, 0, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := poolPressure(tc.acquired, tc.maxConns); got != tc.want {
				t.Errorf("poolPressure(%d, %d) = %v, want %v", tc.acquired, tc.maxConns, got, tc.want)
			}
		})
	}
}
