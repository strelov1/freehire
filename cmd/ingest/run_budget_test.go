package main

import (
	"context"
	"testing"
	"time"
)

func TestRunBudgetFor(t *testing.T) {
	cases := []struct {
		name, timeout, reserve string
		want                   runBudget
		wantErr                bool
	}{
		{name: "unset keeps the crawl unbounded", want: runBudget{reserve: defaultSweepReserve}},
		{name: "timeout with the default reserve", timeout: "3000", want: runBudget{timeout: 3000 * time.Second, reserve: defaultSweepReserve}},
		{name: "explicit reserve", timeout: "4500", reserve: "1200", want: runBudget{timeout: 4500 * time.Second, reserve: 1200 * time.Second}},
		{name: "reserve alone bounds the sweep", reserve: "300", want: runBudget{reserve: 300 * time.Second}},
		{name: "non-numeric timeout", timeout: "50m", wantErr: true},
		{name: "zero timeout", timeout: "0", wantErr: true},
		{name: "negative reserve", timeout: "3000", reserve: "-1", wantErr: true},
		{name: "reserve eats the whole timeout", timeout: "600", reserve: "600", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := runBudgetFor(c.timeout, c.reserve)
			if c.wantErr {
				if err == nil {
					t.Fatalf("runBudgetFor(%q, %q) = %+v, want an error", c.timeout, c.reserve, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("runBudgetFor(%q, %q): %v", c.timeout, c.reserve, err)
			}
			if got != c.want {
				t.Errorf("runBudgetFor(%q, %q) = %+v, want %+v", c.timeout, c.reserve, got, c.want)
			}
		})
	}
}

// The crawl stops a reserve short of the unit's timeout, measured from when the run started.
func TestCrawlContextStopsAReserveShortOfTheTimeout(t *testing.T) {
	started := time.Now()
	b := runBudget{timeout: 3000 * time.Second, reserve: 600 * time.Second}
	ctx, cancel := b.crawlContext(context.Background(), started)
	defer cancel()
	deadline, ok := ctx.Deadline()
	if want := started.Add(2400 * time.Second); !ok || !deadline.Equal(want) {
		t.Errorf("crawl deadline = %v (set=%v), want %v", deadline, ok, want)
	}

	unbounded, cancelUnbounded := runBudget{reserve: 600 * time.Second}.crawlContext(context.Background(), started)
	defer cancelUnbounded()
	if _, ok := unbounded.Deadline(); ok {
		t.Error("a run with no known timeout must not give its crawl a deadline")
	}
}

// The sweep must outlive whatever ended the crawl — its deadline or a signal — and stop no
// later than the moment the unit kills the process anyway.
func TestSweepContextOutlivesTheCrawl(t *testing.T) {
	started := time.Now()
	b := runBudget{timeout: 3000 * time.Second, reserve: 600 * time.Second}
	parent, cancelParent := context.WithCancel(context.Background())

	crawlCtx, cancelCrawl := b.crawlContext(parent, started)
	sweepCtx, cancelSweep := b.sweepContext(parent, started)
	defer cancelSweep()

	cancelCrawl()
	cancelParent() // the signal context: what SIGTERM cancels
	if crawlCtx.Err() == nil {
		t.Fatal("fixture: the crawl context should have ended")
	}
	if err := sweepCtx.Err(); err != nil {
		t.Fatalf("sweep context ended with the crawl: %v", err)
	}
	deadline, ok := sweepCtx.Deadline()
	if want := started.Add(3000 * time.Second); !ok || !deadline.Equal(want) {
		t.Errorf("sweep deadline = %v (set=%v), want the unit's timeout %v", deadline, ok, want)
	}

	// Without a known timeout the reserve bounds it instead, from now.
	unknown, cancelUnknown := runBudget{reserve: 600 * time.Second}.sweepContext(parent, started)
	defer cancelUnknown()
	if unknown.Err() != nil {
		t.Fatal("sweep context with no known timeout ended with its cancelled parent")
	}
	if d, ok := unknown.Deadline(); !ok || time.Until(d) > 600*time.Second {
		t.Errorf("sweep deadline = %v (set=%v), want within the reserve", d, ok)
	}
}
