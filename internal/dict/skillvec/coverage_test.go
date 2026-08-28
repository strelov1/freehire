package skillvec

import (
	"math"
	"sort"
	"testing"
)

// The ordering contract, stated as a test: the feed must read as one descending run of
// coverage — every 100% posting, then every 95%, and so on — with no posting jumping
// above a better-covered one.
//
// This is why the rarity weighting is gone. It was not wrong, it was incompatible: an
// IDF spread of 1..13 is far wider than the gap between 100% and 95%, so a 93% match on
// scarce skills outranked a 100% match on ordinary ones. Measured on production, the
// top forty carried fourteen such inversions, the worst a 33-point drop. No damping
// factor fixed it — 0.05 was clean over forty results and broke by a hundred.
func TestOrderIsStrictlyDescendingCoverage(t *testing.T) {
	// A broad profile, and postings spanning the coverage range at various sizes.
	profile := registry[:60]
	me := ProfileVector(profile)

	type posting struct {
		skills []string
		label  string
	}
	postings := []posting{
		{registry[:20], "20/20"},
		{registry[:8], "8/8"},
		{registry[:6], "6/6"},
		{append(append([]string{}, registry[:19]...), registry[70]), "19/20"},
		{append(append([]string{}, registry[:9]...), registry[71]), "9/10"},
		{append(append([]string{}, registry[:16]...), registry[72:76]...), "16/20"},
		{append(append([]string{}, registry[:6]...), registry[76:80]...), "6/10"},
		{append(append([]string{}, registry[:4]...), registry[80:90]...), "4/14"},
	}

	type scored struct {
		coverage float64
		score    float64
		label    string
	}
	var out []scored
	for _, p := range postings {
		v := JobVector(p.skills)
		if v == nil {
			t.Fatalf("%s built no vector", p.label)
		}
		held := map[string]bool{}
		for _, s := range profile {
			held[s] = true
		}
		var common int
		for _, s := range p.skills {
			if held[s] {
				common++
			}
		}
		out = append(out, scored{float64(common) / float64(len(p.skills)), dot(v, me), p.label})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].score > out[j].score })

	for i := 1; i < len(out); i++ {
		if out[i].coverage > out[i-1].coverage+1e-9 {
			t.Errorf("ranked %s (%.0f%%) above %s (%.0f%%) — the order must never climb back",
				out[i-1].label, out[i-1].coverage*100, out[i].label, out[i].coverage*100)
		}
	}
	if t.Failed() {
		for _, s := range out {
			t.Logf("  %.0f%%  %-6s  score=%.5f", s.coverage*100, s.label, s.score)
		}
	}
}

// Within one coverage band, the posting asking for more is the better match: engaging
// twenty skills you hold beats engaging six.
func TestWithinTheSameCoverageMoreSkillsRanksHigher(t *testing.T) {
	me := ProfileVector(registry[:60])
	twenty := dot(JobVector(registry[:20]), me)
	eight := dot(JobVector(registry[:8]), me)
	if twenty <= eight {
		t.Errorf("20/20 scored %.5f, not above 8/8's %.5f", twenty, eight)
	}
}

// The floor still holds: a one-tag posting is 100% covered and must not lead the feed.
func TestASingleTagPostingStillDoesNotLead(t *testing.T) {
	me := ProfileVector(registry[:60])
	if one, nine := dot(JobVector(registry[:1]), me), dot(JobVector(registry[:9]), me); one >= nine {
		t.Errorf("1/1 scored %.5f, at or above 9/9's %.5f", one, nine)
	}
}

func TestVectorsAreUnitLength(t *testing.T) {
	for name, v := range map[string][]float32{
		"JobVector":     JobVector(registry[:5]),
		"ProfileVector": ProfileVector(registry[:5]),
	} {
		var sum float64
		for _, x := range v {
			sum += float64(x) * float64(x)
		}
		if got := math.Sqrt(sum); math.Abs(got-1) > 1e-5 {
			t.Errorf("%s length = %f, want 1", name, got)
		}
	}
}

func TestOnlyTheJobSideCarriesBallast(t *testing.T) {
	if v := ProfileVector(registry[:2]); v[ballastPosition] != 0 {
		t.Errorf("profile vector set the ballast position to %f, want 0", v[ballastPosition])
	}
	if v := JobVector(registry[:2]); v[ballastPosition] == 0 {
		t.Error("job vector left the ballast position empty")
	}
}

func TestUnusableInputsYieldNoVector(t *testing.T) {
	for name, v := range map[string][]float32{
		"nil skills":        JobVector(nil),
		"empty skills":      JobVector([]string{}),
		"unknown slug only": JobVector([]string{"definitely-not-a-skill"}),
		"profile nil":       ProfileVector(nil),
	} {
		if v != nil {
			t.Errorf("%s produced a vector, want nil", name)
		}
	}
}

func TestUnknownAndRepeatedSlugsAreIgnored(t *testing.T) {
	clean := JobVector(registry[:8])
	noisy := JobVector(append(append([]string{}, registry[:8]...),
		registry[0], registry[1], "retired-slug", "another-unknown"))
	for i := range clean {
		if clean[i] != noisy[i] {
			t.Fatalf("duplicates or unknown slugs changed the vector at position %d", i)
		}
	}
}

func dot(a, b []float32) float64 {
	var s float64
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}
