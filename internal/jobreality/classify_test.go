package jobreality

import (
	"testing"
	"time"
)

var now = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// base is an ordinary fresh, unique, active posting. Tests mutate one axis.
func base() Input {
	return Input{
		Now:              now,
		CreatedAt:        now.Add(-2 * 24 * time.Hour),
		PostedAt:         now.Add(-2 * 24 * time.Hour),
		HasPostedAt:      true,
		RepostCount:      1,
		MassPostingCount: 1,
		EvergreenText:    false,
	}
}

func daysAgo(d int) time.Time { return now.Add(-time.Duration(d) * 24 * time.Hour) }

func TestClassify_FreshWhenNewAndNoSignals(t *testing.T) {
	res := Classify(base())
	if res.Class != ClassFresh {
		t.Errorf("class = %q, want %q", res.Class, ClassFresh)
	}
	if res.Evidence.AgeDays != 2 {
		t.Errorf("ageDays = %d, want 2", res.Evidence.AgeDays)
	}
}

func TestClassify_StaleWhenLongOpenButNoOtherSignal(t *testing.T) {
	in := base()
	in.CreatedAt, in.PostedAt = daysAgo(240), daysAgo(240)
	res := Classify(in)
	if res.Class != ClassStale {
		t.Errorf("class = %q, want %q", res.Class, ClassStale)
	}
}

// Age alone must NEVER reach the verdict — a genuinely hard-to-fill senior role open
// a long time is not evergreen.
func TestClassify_AgeAloneIsNotEvergreen(t *testing.T) {
	in := base()
	in.CreatedAt, in.PostedAt = daysAgo(400), daysAgo(400)
	if res := Classify(in); res.Class == ClassLikelyEvergreen {
		t.Errorf("age alone reached %q", ClassLikelyEvergreen)
	}
}

func TestClassify_LikelyEvergreenOnConvergence(t *testing.T) {
	in := base()
	in.CreatedAt, in.PostedAt = daysAgo(240), daysAgo(240)
	in.RepostCount = 6
	in.EvergreenText = true
	res := Classify(in)
	if res.Class != ClassLikelyEvergreen {
		t.Errorf("class = %q, want %q", res.Class, ClassLikelyEvergreen)
	}
	if res.Evidence.RepostCount != 6 {
		t.Errorf("evidence repostCount = %d, want 6", res.Evidence.RepostCount)
	}
}

// Two independent signals (old age + mass-posting) are enough — the convergence gate
// is exactly two. MassPostingCount is a subset of RepostCount (open ⊆ any-status), so
// a valid all-concurrent spray sets both equal; the repost signal (historical churn)
// stays silent here, proving mass-posting counts as one signal, not two.
func TestClassify_TwoSignalsConverge(t *testing.T) {
	in := base()
	in.CreatedAt, in.PostedAt = daysAgo(120), daysAgo(120)
	in.RepostCount, in.MassPostingCount = 8, 8
	if res := Classify(in); res.Class != ClassLikelyEvergreen {
		t.Errorf("class = %q, want %q (old age + mass-posting)", res.Class, ClassLikelyEvergreen)
	}
}

// A concurrent mass-posting on its own — no age, no history, no text — must NOT reach
// the verdict: mass-posting is a single signal and the gate needs convergence.
func TestClassify_MassPostingAloneIsNotEvergreen(t *testing.T) {
	in := base() // age 2 days, RepostCount default bumped to match mass below
	in.RepostCount, in.MassPostingCount = 8, 8
	if res := Classify(in); res.Class == ClassLikelyEvergreen {
		t.Errorf("mass-posting alone reached %q", ClassLikelyEvergreen)
	}
}

// A recent posted date over an old first-seen is recorded as fake-freshness evidence,
// but on its own (only the old-age signal) it does not brand the job evergreen.
func TestClassify_FakeFreshnessRecordedNotVerdict(t *testing.T) {
	in := base()
	in.CreatedAt = daysAgo(240)
	in.PostedAt, in.HasPostedAt = daysAgo(1), true
	res := Classify(in)
	if !res.Evidence.FakeFreshness {
		t.Error("expected FakeFreshness evidence when posted recent but first-seen old")
	}
	if res.Class == ClassLikelyEvergreen {
		t.Errorf("fake-freshness alone reached %q", ClassLikelyEvergreen)
	}
}

func TestClassify_NoFakeFreshnessWhenPostedMatchesFirstSeen(t *testing.T) {
	in := base()
	in.CreatedAt, in.PostedAt = daysAgo(240), daysAgo(240)
	if res := Classify(in); res.Evidence.FakeFreshness {
		t.Error("did not expect FakeFreshness when posted date matches first-seen")
	}
}

func TestClassify_Deterministic(t *testing.T) {
	in := base()
	in.CreatedAt = daysAgo(200)
	in.RepostCount, in.MassPostingCount, in.EvergreenText = 4, 6, true
	if Classify(in) != Classify(in) {
		t.Error("classification not deterministic for identical input")
	}
}
