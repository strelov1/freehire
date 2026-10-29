package main

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/strelov1/freehire/internal/job/liveness"
)

// recordingPass is a pass that answers Uncertain (so no DB write is issued) and records which
// candidates it was asked about.
func recordingPass(source string, maxPerRun int) (pacedPass, func() []int64) {
	var mu sync.Mutex
	var seen []int64
	p := pacedPass{
		source:    source,
		interval:  1,
		maxPerRun: maxPerRun,
		check: func(_ context.Context, c candidate) pacedVerdict {
			mu.Lock()
			defer mu.Unlock()
			seen = append(seen, c.id)
			return pacedVerdict{verdict: liveness.Uncertain}
		},
	}
	return p, func() []int64 {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(seen)
	}
}

// A close takes two consecutive Expired reads, so a row already struck once must be read
// again on the next run rather than wait for the shuffle to reach it on a backlog several
// runs deep.
func TestApplyPacedReadsStruckCandidatesFirst(t *testing.T) {
	cands := make([]candidate, 50)
	for i := range cands {
		cands[i] = candidate{id: int64(i), source: "djinni"}
	}
	cands[17].strikes = 1
	cands[42].strikes = 1
	p, seen := recordingPass("djinni", 2)
	var st stats
	st.applyPaced(context.Background(), nil, cands, p)
	got := seen()
	slices.Sort(got)
	if want := []int64{17, 42}; !slices.Equal(got, want) {
		t.Errorf("a run bounded to 2 read %v, want the struck %v", got, want)
	}
}

// Each pass reads only its own source's candidates.
func TestApplyPacedPassesRoutesEachSourceToItsPass(t *testing.T) {
	djinni, djinniSeen := recordingPass("djinni", 10)
	torre, torreSeen := recordingPass("torre", 10)
	bySource := map[string][]candidate{
		"djinni": {{id: 1, source: "djinni"}, {id: 2, source: "djinni"}},
		"torre":  {{id: 3, source: "torre"}},
	}
	var st stats
	st.applyPacedPasses(context.Background(), nil, bySource, []pacedPass{djinni, torre})
	gotDjinni := djinniSeen()
	slices.Sort(gotDjinni)
	if want := []int64{1, 2}; !slices.Equal(gotDjinni, want) {
		t.Errorf("djinni pass read %v, want %v", gotDjinni, want)
	}
	if got, want := torreSeen(), []int64{3}; !slices.Equal(got, want) {
		t.Errorf("torre pass read %v, want %v", got, want)
	}
	if n := st.probed.Load(); n != 3 {
		t.Errorf("probed = %d, want 3", n)
	}
}

// Every probeDespiteRegistered member reaches exactly one evidence path. One that reached
// none would be logged and skipped every run — the silent omission this worker exists to
// avoid — and one that reached two would be probed twice and struck twice in one run.
func TestEveryProbedSourceHasExactlyOneEvidencePath(t *testing.T) {
	paced := pacedPasses(nil)
	for _, src := range probeDespiteRegistered {
		paths := 0
		if slices.Contains(probeDespiteRegisteredGET, src) {
			paths++
		}
		if src == "himalayas" || src == "echojobs" {
			paths++
		}
		if slices.ContainsFunc(paced, func(p pacedPass) bool { return p.source == src }) {
			paths++
		}
		if paths != 1 {
			t.Errorf("%q reaches %d evidence paths, want 1", src, paths)
		}
	}
}
