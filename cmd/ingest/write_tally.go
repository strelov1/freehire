package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// writeTally counts, per provider, which of the two ingest writes each persisted posting took:
// the cheap liveness refresh (RefreshUnchangedJob) or the full upsert.
//
// The saving this change delivers is proportional to the cheap share, and nothing else measures
// it. jobhash.Of covers 19 fields, and a provider only has to vary ONE of them between crawls —
// a session token in the url, a posted time the board re-serializes, unstable whitespace in a
// location — for its rows to keep taking the full write forever. That failure is silent: the
// crawl succeeds, the counts look ordinary, and the only symptom is a saving that never
// arrives. A provider sitting at 0% is a finding worth taking to its adapter, since the same
// churn has also been forcing a pointless search-index push on every crawl.
//
// Safe for concurrent Save calls (boards run in parallel goroutines), like crawledSet.
type writeTally struct {
	mu    sync.Mutex
	cheap map[string]int
	total map[string]int
}

func newWriteTally() *writeTally {
	return &writeTally{cheap: map[string]int{}, total: map[string]int{}}
}

// record notes one persisted posting. A nil tally is inert, so a Store built without one — as
// the tests that do not care about it are — needs no guard at the call site.
func (w *writeTally) record(provider string, tookCheapWrite bool) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.total[provider]++
	if tookCheapWrite {
		w.cheap[provider]++
	}
}

// summary renders one line's worth of per-provider shares, sorted so two runs of the same board
// file compare directly. Empty when the run persisted nothing, so the caller can omit the line
// rather than print a label with no result behind it.
func (w *writeTally) summary() string {
	if w == nil {
		return ""
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	providers := make([]string, 0, len(w.total))
	for p := range w.total {
		providers = append(providers, p)
	}
	sort.Strings(providers)

	parts := make([]string, 0, len(providers))
	for _, p := range providers {
		total := w.total[p]
		parts = append(parts, fmt.Sprintf("%s cheap=%d/%d (%d%%)", p, w.cheap[p], total, w.cheap[p]*100/total))
	}
	return strings.Join(parts, " ")
}
