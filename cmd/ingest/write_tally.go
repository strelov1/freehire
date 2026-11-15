package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// writeTally counts, per provider, which of the two ingest writes each persisted posting took: the cheap liveness refresh (RefreshUnchangedJob) or the full upsert.
//
// The cheap share is the only measure of the saving. jobhash.Of covers 19 fields, and a provider that varies ONE of them between crawls (a session token in the url, a re-serialized posted time, unstable whitespace) keeps taking the full write silently, and forces a pointless search-index push each crawl. A provider at 0% is worth taking to its adapter.
//
// Safe for concurrent Save calls (boards run in parallel goroutines), like crawledSet.
type writeTally struct {
	mu    sync.Mutex
	cheap map[string]int
	total map[string]int
	// fenced counts attempts a concurrent state change refused (job_activity's state fence), so a
	// provider that keeps racing its own closers or rekeys shows up in the run's one summary line.
	fenced map[string]int
}

func newWriteTally() *writeTally {
	return &writeTally{cheap: map[string]int{}, total: map[string]int{}, fenced: map[string]int{}}
}

// recordFenced notes one fenced attempt. Nil-safe, like record.
func (w *writeTally) recordFenced(provider string) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.fenced[provider]++
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

// summary renders one line of per-provider shares, sorted so runs compare directly. Empty when nothing was persisted, so the caller can omit the line.
func (w *writeTally) summary() string {
	if w == nil {
		return ""
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	seen := map[string]bool{}
	providers := make([]string, 0, len(w.total))
	for p := range w.total {
		seen[p] = true
		providers = append(providers, p)
	}
	for p := range w.fenced {
		if !seen[p] {
			providers = append(providers, p)
		}
	}
	sort.Strings(providers)

	parts := make([]string, 0, len(providers))
	for _, p := range providers {
		part := p
		if total := w.total[p]; total > 0 {
			part = fmt.Sprintf("%s cheap=%d/%d (%d%%)", p, w.cheap[p], total, w.cheap[p]*100/total)
		}
		if n := w.fenced[p]; n > 0 {
			part += fmt.Sprintf(" fenced=%d", n)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, " ")
}
